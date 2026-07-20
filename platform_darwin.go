//go:build darwin

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	exeName         = "CopyPairFolder"
	serviceCopyName = "复制并新建文件夹.workflow"
	serviceCutName  = "剪切并新建文件夹.workflow"
	installSubdir   = "CopyPairFolder"
)

func helpText() string {
	return appName + "\n\n" +
		"双击本程序 = 安装 Finder 快速操作（右键菜单）\n" +
		"卸载：CopyPairFolder --uninstall\n\n" +
		"快速操作两项：\n" +
		"· 「" + menuCopyCaption + "」— 配对复制到新文件夹，源文件保留\n" +
		"· 「" + menuCutCaption + "」— 配对剪切到新文件夹，源文件删除\n\n" +
		"仅处理「同名且同时存在 JPG 与 CR3」的配对。\n" +
		"适用于 Apple Silicon（M3 / M4 / M5 等）。"
}

func selectionSettleDelay() time.Duration { return 400 * time.Millisecond }

func messageBox(text, caption string) {
	script := fmt.Sprintf(`display dialog %s with title %s buttons {"好"} default button 1 with icon note`,
		appleScriptString(text), appleScriptString(caption))
	_ = exec.Command("osascript", "-e", script).Run()
}

func confirmDialog(text, caption string) bool {
	script := fmt.Sprintf(`display dialog %s with title %s buttons {"取消", "继续"} default button "继续" with icon caution`,
		appleScriptString(text), appleScriptString(caption))
	out, err := exec.Command("osascript", "-e", script).CombinedOutput()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), "继续")
}

func appleScriptString(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return `"` + s + `"`
}

func appSupportDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "Application Support", installSubdir)
}

func servicesDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "Services")
}

func doInstall() {
	dstDir := appSupportDir()
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		messageBox("安装失败：无法创建目录\n"+err.Error(), appName)
		os.Exit(1)
	}

	self, err := os.Executable()
	if err != nil {
		messageBox("安装失败：无法定位自身\n"+err.Error(), appName)
		os.Exit(1)
	}
	self, _ = filepath.Abs(self)
	dstBin := filepath.Join(dstDir, exeName)

	if !sameFile(self, dstBin) {
		if err := copyFile(self, dstBin); err != nil {
			messageBox("安装失败：无法复制程序\n"+err.Error(), appName)
			os.Exit(1)
		}
	}
	_ = os.Chmod(dstBin, 0755)

	if err := installFinderService(dstBin, serviceCopyName, menuCopyCaption, "/copy"); err != nil {
		messageBox("安装失败：无法写入 Finder 服务（复制）\n"+err.Error(), appName)
		os.Exit(1)
	}
	if err := installFinderService(dstBin, serviceCutName, menuCutCaption, "/cut"); err != nil {
		messageBox("安装失败：无法写入 Finder 服务（剪切）\n"+err.Error(), appName)
		os.Exit(1)
	}

	_ = exec.Command("/System/Library/CoreServices/pbs", "-flush").Run()

	messageBox("安装成功！（Apple Silicon：M3 / M4 / M5）\n\n"+
		"用法：\n"+
		"1. 在 Finder 中选中 JPG / CR3\n"+
		"2. 右键 → 快速操作，可见两项：\n"+
		"   · 「"+menuCopyCaption+"」\n"+
		"   · 「"+menuCutCaption+"」\n"+
		"   （若没有：右键 → 服务）\n\n"+
		"若仍看不到：系统设置 → 键盘 → 键盘快捷键 → 服务，勾选这两项。\n\n"+
		"卸载：\n"+dstBin+" --uninstall", appName)
}

func doUninstall() {
	_ = os.RemoveAll(filepath.Join(servicesDir(), serviceCopyName))
	_ = os.RemoveAll(filepath.Join(servicesDir(), serviceCutName))
	_ = os.RemoveAll(appSupportDir())
	_ = exec.Command("/System/Library/CoreServices/pbs", "-flush").Run()
	messageBox("已卸载「"+menuCopyCaption+"」「"+menuCutCaption+"」。", appName)
}

func installFinderService(binPath, serviceName, caption, modeFlag string) error {
	wfRoot := filepath.Join(servicesDir(), serviceName)
	contents := filepath.Join(wfRoot, "Contents")
	_ = os.RemoveAll(wfRoot)
	if err := os.MkdirAll(contents, 0755); err != nil {
		return err
	}

	info := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>NSServices</key>
	<array>
		<dict>
			<key>NSMenuItem</key>
			<dict>
				<key>default</key>
				<string>` + caption + `</string>
			</dict>
			<key>NSMessage</key>
			<string>runWorkflowAsService</string>
			<key>NSRequiredContext</key>
			<dict>
				<key>NSApplicationIdentifier</key>
				<string>com.apple.finder</string>
			</dict>
			<key>NSSendFileTypes</key>
			<array>
				<string>public.item</string>
			</array>
		</dict>
	</array>
</dict>
</plist>
`
	if err := os.WriteFile(filepath.Join(contents, "Info.plist"), []byte(info), 0644); err != nil {
		return err
	}

	escapedBin := strings.ReplaceAll(binPath, `'`, `'\''`)
	// 不同服务用不同 UUID，避免 Automator 缓存冲突（须为合法十六进制）
	inUUID, outUUID, actUUID := "A1B2C3D4-E5F6-7890-ABCD-EF1234567890", "B2C3D4E5-F6A7-8901-BCDE-F12345678901", "C3D4E5F6-A7B8-9012-CDEF-123456789012"
	if modeFlag == "/cut" {
		inUUID, outUUID, actUUID = "A1B2C3D4-E5F6-7890-ABCD-EF12345678A1", "B2C3D4E5-F6A7-8901-BCDE-F123456789A2", "C3D4E5F6-A7B8-9012-CDEF-1234567890A3"
	}
	wflow := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>AMApplicationBuild</key>
	<string>523</string>
	<key>AMApplicationVersion</key>
	<string>2.10</string>
	<key>AMDocumentVersion</key>
	<string>2</string>
	<key>actions</key>
	<array>
		<dict>
			<key>action</key>
			<dict>
				<key>AMAccepts</key>
				<dict>
					<key>Container</key>
					<string>List</string>
					<key>Optional</key>
					<true/>
					<key>Types</key>
					<array>
						<string>com.apple.cocoa.path</string>
					</array>
				</dict>
				<key>AMActionVersion</key>
				<string>2.0.3</string>
				<key>AMApplication</key>
				<array>
					<string>Automator</string>
				</array>
				<key>AMParameterProperties</key>
				<dict>
					<key>COMMAND_STRING</key>
					<dict/>
					<key>CheckedForUserDefaultShell</key>
					<dict/>
					<key>inputMethod</key>
					<dict/>
					<key>shell</key>
					<dict/>
					<key>source</key>
					<dict/>
				</dict>
				<key>AMProvides</key>
				<dict>
					<key>Container</key>
					<string>List</string>
					<key>Types</key>
					<array>
						<string>com.apple.cocoa.string</string>
					</array>
				</dict>
				<key>ActionBundlePath</key>
				<string>/System/Library/Automator/Run Shell Script.action</string>
				<key>ActionName</key>
				<string>Run Shell Script</string>
				<key>ActionParameters</key>
				<dict>
					<key>COMMAND_STRING</key>
					<string>'` + escapedBin + `' ` + modeFlag + ` "$@" &amp;</string>
					<key>CheckedForUserDefaultShell</key>
					<true/>
					<key>inputMethod</key>
					<integer>1</integer>
					<key>shell</key>
					<string>/bin/bash</string>
					<key>source</key>
					<string></string>
				</dict>
				<key>BundleIdentifier</key>
				<string>com.apple.RunShellScript</string>
				<key>CFBundleVersion</key>
				<string>2.0.3</string>
				<key>CanShowSelectedItemsWhenRun</key>
				<false/>
				<key>CanShowWhenRun</key>
				<true/>
				<key>Category</key>
				<array>
					<string>AMCategoryUtilities</string>
				</array>
				<key>Class Name</key>
				<string>RunShellScriptAction</string>
				<key>InputUUID</key>
				<string>` + inUUID + `</string>
				<key>Keywords</key>
				<array>
					<string>Shell</string>
					<string>Script</string>
					<string>Command</string>
					<string>Run</string>
					<string>Unix</string>
				</array>
				<key>OutputUUID</key>
				<string>` + outUUID + `</string>
				<key>UUID</key>
				<string>` + actUUID + `</string>
				<key>UnlocalizedApplications</key>
				<array>
					<string>Automator</string>
				</array>
				<key>arguments</key>
				<dict>
					<key>0</key>
					<dict>
						<key>default value</key>
						<integer>0</integer>
						<key>name</key>
						<string>inputMethod</string>
						<key>required</key>
						<string>0</string>
						<key>type</key>
						<string>0</string>
						<key>uuid</key>
						<string>0</string>
					</dict>
					<key>1</key>
					<dict>
						<key>default value</key>
						<false/>
						<key>name</key>
						<string>CheckedForUserDefaultShell</string>
						<key>required</key>
						<string>0</string>
						<key>type</key>
						<string>0</string>
						<key>uuid</key>
						<string>1</string>
					</dict>
					<key>2</key>
					<dict>
						<key>default value</key>
						<string></string>
						<key>name</key>
						<string>COMMAND_STRING</string>
						<key>required</key>
						<string>0</string>
						<key>type</key>
						<string>0</string>
						<key>uuid</key>
						<string>2</string>
					</dict>
					<key>3</key>
					<dict>
						<key>default value</key>
						<string>/bin/sh</string>
						<key>name</key>
						<string>shell</string>
						<key>required</key>
						<string>0</string>
						<key>type</key>
						<string>0</string>
						<key>uuid</key>
						<string>3</string>
					</dict>
				</dict>
				<key>isViewVisible</key>
				<true/>
				<key>location</key>
				<string>309.000000:253.000000</string>
				<key>nibPath</key>
				<string>/System/Library/Automator/Run Shell Script.action/Contents/Resources/Base.lproj/main.nib</string>
			</dict>
			<key>isViewVisible</key>
			<true/>
		</dict>
	</array>
	<key>connectors</key>
	<dict/>
	<key>workflowMetaData</key>
	<dict>
		<key>serviceInputTypeIdentifier</key>
		<string>com.apple.Automator.fileSystemObject</string>
		<key>serviceOutputTypeIdentifier</key>
		<string>com.apple.Automator.nothing</string>
		<key>serviceProcessesInput</key>
		<integer>0</integer>
		<key>workflowTypeIdentifier</key>
		<string>com.apple.Automator.servicesMenu</string>
	</dict>
</dict>
</plist>
`
	return os.WriteFile(filepath.Join(contents, "document.wflow"), []byte(wflow), 0644)
}

func stateDir() string {
	d := filepath.Join(appSupportDir(), "run")
	_ = os.MkdirAll(d, 0755)
	return d
}

func tryBecomeLeader(sessionFile string, mode transferMode) bool {
	_ = mode
	if st, err := os.Stat(sessionFile); err == nil {
		if time.Since(st.ModTime()) < sessionCooldown {
			return false
		}
		_ = os.Remove(sessionFile)
	}
	return claimSessionFile(sessionFile)
}

func getSelection(hint string) []string {
	script := `
try
	tell application "Finder"
		set selList to selection as alias list
		set pathText to ""
		repeat with f in selList
			set pathText to pathText & (POSIX path of f) & linefeed
		end repeat
		return pathText
	end tell
on error
	return ` + appleScriptString(hint) + `
end try
`
	out, err := exec.Command("osascript", "-e", script).Output()
	if err != nil {
		return []string{hint}
	}
	var res []string
	for _, ln := range strings.Split(string(out), "\n") {
		ln = strings.TrimSpace(ln)
		if ln != "" {
			res = append(res, ln)
		}
	}
	if len(res) == 0 {
		return []string{hint}
	}
	return res
}

func openAndRenameFolder(folder string) {
	_ = exec.Command("open", "-R", folder).Start()
	script := `
delay 0.6
tell application "Finder"
	activate
	try
		set theItem to (POSIX file ` + appleScriptString(folder) + `) as alias
		select theItem
	end try
end tell
`
	_ = exec.Command("osascript", "-e", script).Start()
}
