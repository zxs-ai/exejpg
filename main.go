package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	appName       = "配对文件夹工具"
	menuCopyCaption = "复制并新建文件夹"
	menuCutCaption  = "剪切并新建文件夹"
	// Finder / 资源管理器多选时常按文件逐个启动；同一操作窗口期内只允许跑一次
	sessionCooldown = 8 * time.Second
)

type transferMode int

const (
	modeCopy transferMode = iota
	modeCut
)

func (m transferMode) String() string {
	if m == modeCut {
		return "cut"
	}
	return "copy"
}

func (m transferMode) verb() string {
	if m == modeCut {
		return "剪切"
	}
	return "复制"
}

func (m transferMode) menuCaption() string {
	if m == modeCut {
		return menuCutCaption
	}
	return menuCopyCaption
}

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		doInstall()
		return
	}

	switch strings.ToLower(args[0]) {
	case "/install", "-install", "--install":
		doInstall()
	case "/uninstall", "-uninstall", "--uninstall":
		doUninstall()
	case "/embedding", "-embedding":
		// 兼容旧版 Windows COM 注册；新版以经典右键为主
		runCOMServer()
	case "/process-list":
		mode, rest := parseModeFlag(args[1:])
		if len(rest) >= 1 {
			runFromList(rest[0], mode)
		}
	case "/copy", "-copy", "--copy":
		runTransfer(args[1:], modeCopy)
	case "/cut", "-cut", "--cut":
		runTransfer(args[1:], modeCut)
	case "/help", "-h", "--help":
		showHelp()
	default:
		// 无显式模式时：兼容旧版右键（仅复制）
		mode, rest := parseModeFlag(args)
		runTransfer(rest, mode)
	}
}

func parseModeFlag(args []string) (transferMode, []string) {
	if len(args) == 0 {
		return modeCopy, args
	}
	switch strings.ToLower(args[0]) {
	case "/copy", "-copy", "--copy", "copy":
		return modeCopy, args[1:]
	case "/cut", "-cut", "--cut", "cut":
		return modeCut, args[1:]
	default:
		return modeCopy, args
	}
}

func showHelp() {
	messageBox(helpText(), appName)
}

func runTransfer(args []string, mode transferMode) {
	if len(args) == 0 {
		os.Exit(0)
	}
	clicked := strings.TrimSpace(args[0])
	if clicked == "" {
		os.Exit(0)
	}
	clicked, _ = filepath.Abs(clicked)

	listFile := filepath.Join(stateDir(), "pending_"+mode.String()+".txt")
	sessionFile := filepath.Join(stateDir(), "session_"+mode.String()+".active")

	appendPending(listFile, clicked)

	if !tryBecomeLeader(sessionFile, mode) {
		os.Exit(0)
	}
	defer touchSession(sessionFile)

	time.Sleep(selectionSettleDelay())

	selected := getSelection(clicked)
	selected = append(selected, readPending(listFile)...)
	selected = append(selected, collectArgsFiles(args)...)
	_ = os.Remove(listFile)

	files := normalizeFiles(selected)
	processSelected(files, mode)
}

func runFromList(listPath string, mode transferMode) {
	defer os.Remove(listPath)
	data, err := os.ReadFile(listPath)
	if err != nil {
		messageBox("读取选中文件失败：\n"+err.Error(), mode.menuCaption())
		return
	}
	var selected []string
	for _, line := range strings.Split(strings.TrimPrefix(string(data), "\ufeff"), "\n") {
		if line = strings.TrimSpace(strings.TrimRight(line, "\r")); line != "" {
			selected = append(selected, line)
		}
	}
	processSelected(normalizeFiles(selected), mode)
}

func processSelected(files []string, mode transferMode) {
	caption := mode.menuCaption()
	if len(files) == 0 {
		messageBox("未获取到选中的文件。\n\n请重新选中后再试。", caption)
		return
	}

	dir := filepath.Dir(files[0])
	pairs := findJpgCr3Pairs(files, dir)
	if len(pairs) == 0 {
		messageBox("未找到同时具备 JPG 与 CR3 的同名配对。\n\n只有一种格式的文件不会"+mode.verb()+"。", caption)
		return
	}

	totalFiles := len(pairs) * 2
	if mode == modeCut {
		msg := fmt.Sprintf("将把 %d 组配对（共 %d 个文件）剪切到新文件夹。\n\n原位置的这些文件会被删除，是否继续？",
			len(pairs), totalFiles)
		if !confirmDialog(msg, caption) {
			return
		}
	}

	folderName := "配对导出_" + time.Now().Format("20060102_150405")
	destDir := uniqueDir(filepath.Join(dir, folderName))
	if err := os.MkdirAll(destDir, 0755); err != nil {
		messageBox("创建文件夹失败：\n"+err.Error(), caption)
		return
	}

	totalBytes := int64(0)
	var sources []string
	for _, paths := range pairs {
		for _, src := range paths {
			sources = append(sources, src)
			if st, err := os.Stat(src); err == nil {
				totalBytes += st.Size()
			}
		}
	}

	progress := startProgressWindow(totalFiles, totalBytes, mode)
	defer progress.Close()

	done := 0
	var doneBytes int64
	for _, src := range sources {
		dst := filepath.Join(destDir, filepath.Base(src))
		name := filepath.Base(src)
		progress.Update(done, doneBytes, name)
		_, err := copyFileWithProgress(src, dst, func(delta int64) {
			doneBytes += delta
			progress.Update(done, doneBytes, name)
		})
		if err != nil {
			messageBox(mode.verb()+"失败：\n"+err.Error(), caption)
			return
		}
		done++
		progress.Update(done, doneBytes, "")
	}

	if mode == modeCut {
		var failed []string
		for _, src := range sources {
			if err := os.Remove(src); err != nil {
				failed = append(failed, filepath.Base(src)+"："+err.Error())
			}
		}
		if len(failed) > 0 {
			progress.Finish(done)
			openAndRenameFolder(destDir)
			messageBox(fmt.Sprintf("文件已写入新文件夹，但部分源文件删除失败：\n\n%s\n\n新文件夹：%s",
				strings.Join(failed, "\n"), destDir), caption)
			return
		}
	}

	progress.Finish(done)
	openAndRenameFolder(destDir)
	action := "复制"
	if mode == modeCut {
		action = "剪切（已从原位置删除）"
	}
	messageBox(fmt.Sprintf("完成！\n\n配对组数：%d\n%s文件：%d\n新文件夹：%s",
		len(pairs), action, done, destDir), caption)
}

func claimSessionFile(sessionFile string) bool {
	if st, err := os.Stat(sessionFile); err == nil {
		if time.Since(st.ModTime()) < sessionCooldown {
			return false
		}
		_ = os.Remove(sessionFile)
	}
	f, err := os.OpenFile(sessionFile, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return false
	}
	_, _ = f.WriteString(fmt.Sprintf("%d\n", os.Getpid()))
	_ = f.Close()
	return true
}

func touchSession(sessionFile string) {
	_ = os.WriteFile(sessionFile, []byte(fmt.Sprintf("%d\n", time.Now().UnixNano())), 0644)
}

func appendPending(listFile, path string) {
	f, err := os.OpenFile(listFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(path + "\n")
}

func readPending(listFile string) []string {
	data, err := os.ReadFile(listFile)
	if err != nil {
		return nil
	}
	var out []string
	for _, ln := range strings.Split(string(data), "\n") {
		ln = strings.TrimSpace(ln)
		if ln != "" {
			out = append(out, ln)
		}
	}
	return out
}

func uniqueDir(path string) string {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return path
	}
	for i := 2; i < 1000; i++ {
		cand := fmt.Sprintf("%s_%d", path, i)
		if _, err := os.Stat(cand); os.IsNotExist(err) {
			return cand
		}
	}
	return fmt.Sprintf("%s_%d", path, time.Now().UnixNano())
}

func collectArgsFiles(args []string) []string {
	out := make([]string, 0, len(args))
	for _, a := range args {
		a = strings.TrimSpace(a)
		if a == "" {
			continue
		}
		low := strings.ToLower(a)
		if low == "/copy" || low == "-copy" || low == "--copy" ||
			low == "/cut" || low == "-cut" || low == "--cut" ||
			low == "copy" || low == "cut" {
			continue
		}
		out = append(out, a)
	}
	return out
}

func normalizeFiles(paths []string) []string {
	files := make([]string, 0, len(paths))
	seen := map[string]struct{}{}
	for _, p := range paths {
		p = strings.TrimSpace(strings.Trim(p, `"'`))
		p = strings.TrimRight(p, "\r\n")
		if p == "" {
			continue
		}
		p, _ = filepath.Abs(p)
		key := strings.ToLower(p)
		if _, ok := seen[key]; ok {
			continue
		}
		fi, err := os.Stat(p)
		if err != nil || fi.IsDir() {
			continue
		}
		seen[key] = struct{}{}
		files = append(files, p)
	}
	return files
}

func isJpgExt(ext string) bool {
	switch strings.ToLower(ext) {
	case ".jpg", ".jpeg":
		return true
	default:
		return false
	}
}

func isCr3Ext(ext string) bool {
	return strings.ToLower(ext) == ".cr3"
}

func findJpgCr3Pairs(selected []string, dir string) map[string][]string {
	type bucket struct {
		jpg string
		cr3 string
	}

	selectedSet := map[string]struct{}{}
	for _, p := range selected {
		selectedSet[strings.ToLower(p)] = struct{}{}
	}

	byBase := map[string]*bucket{}
	ensure := func(base string) *bucket {
		if byBase[base] == nil {
			byBase[base] = &bucket{}
		}
		return byBase[base]
	}

	classify := func(p string) {
		ext := filepath.Ext(p)
		base := strings.ToLower(strings.TrimSuffix(filepath.Base(p), ext))
		b := ensure(base)
		if isJpgExt(ext) {
			b.jpg = p
		} else if isCr3Ext(ext) {
			b.cr3 = p
		}
	}

	for _, p := range selected {
		classify(p)
	}

	entries, _ := os.ReadDir(dir)
	indexByBase := map[string]map[string]string{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		ext := filepath.Ext(name)
		base := strings.ToLower(strings.TrimSuffix(name, ext))
		full := filepath.Join(dir, name)
		if indexByBase[base] == nil {
			indexByBase[base] = map[string]string{}
		}
		if isJpgExt(ext) {
			indexByBase[base]["jpg"] = full
		} else if isCr3Ext(ext) {
			indexByBase[base]["cr3"] = full
		}
	}

	for base, b := range byBase {
		if info, ok := indexByBase[base]; ok {
			if b.jpg == "" {
				b.jpg = info["jpg"]
			}
			if b.cr3 == "" {
				b.cr3 = info["cr3"]
			}
		}
	}

	out := map[string][]string{}
	for base, b := range byBase {
		if b.jpg == "" || b.cr3 == "" {
			continue
		}
		_, jpgSel := selectedSet[strings.ToLower(b.jpg)]
		_, cr3Sel := selectedSet[strings.ToLower(b.cr3)]
		if !jpgSel && !cr3Sel {
			continue
		}
		out[base] = []string{b.jpg, b.cr3}
	}
	return out
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

func copyFileWithProgress(src, dst string, onProgress func(int64)) (int64, error) {
	in, err := os.Open(src)
	if err != nil {
		return 0, err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return 0, err
	}
	defer out.Close()

	buf := make([]byte, 1024*1024)
	var total int64
	for {
		n, readErr := in.Read(buf)
		if n > 0 {
			written, writeErr := out.Write(buf[:n])
			total += int64(written)
			if written > 0 && onProgress != nil {
				onProgress(int64(written))
			}
			if writeErr != nil {
				return total, writeErr
			}
			if written != n {
				return total, io.ErrShortWrite
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return total, readErr
		}
	}
	return total, out.Close()
}

func sameFile(a, b string) bool {
	a, _ = filepath.Abs(a)
	b, _ = filepath.Abs(b)
	return strings.EqualFold(a, b)
}
