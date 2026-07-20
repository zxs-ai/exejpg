# 配对文件夹工具（CopyPairFolder / exejpg）

资源管理器 / Finder 右键工具：选中图片后，一键把**同名且同时具备 JPG 与 CR3** 的配对文件放入当前目录下的新文件夹，并定位该文件夹方便重命名。

支持两项菜单，可同时使用：

- **复制并新建文件夹** — 配对复制，源文件保留  
- **剪切并新建文件夹** — 配对剪切，成功后删除源文件  

仓库：https://github.com/zxs-ai/exejpg  
关联审片 App：[图像管理工具 ImgMan](https://github.com/zxs-ai/ImgMan)

---

## 功能说明

| 项目 | 说明 |
|------|------|
| 菜单名 | **复制并新建文件夹**、**剪切并新建文件夹** |
| 系统 | **Windows 10 / 11（x64）**、**macOS Apple Silicon（M3 / M4 / M5，arm64）** |
| 配对规则 | 同名文件同时存在 `.jpg` / `.jpeg` **和** `.cr3` 才处理 |
| 扩展名大小写 | 支持 `.jpg` / `.JPG` / `.jpeg` / `.JPEG` / `.cr3` / `.CR3` |
| 单格式文件 | 只有 JPG 或只有 CR3 → **不处理** |
| 新文件夹位置 | 选中文件所在的**当前目录** |
| 多选 | 支持；合并选中项并只处理一次（防多开） |
| 剪切确认 | 剪切前弹出确认；先写入新文件夹成功后再删源文件 |
| 进度提示 | Windows 有进度窗；macOS 有通知 + 完成对话框 |
| 完成后 | 打开并选中新文件夹（便于 F2 / 回车改名） |
| 右键位置 | Windows 注册 `Position=Top`，尽量靠前显示 |
| 安装体验（Win） | 双击后仅显示「正在安装」忙碌窗；完成后**一个**成功提示（含位置 / 用法 / 卸载） |

---

## 下载

| 平台 | 文件 |
|------|------|
| Windows | [`CopyPairFolder.exe`](./CopyPairFolder.exe) |
| macOS Apple Silicon | [`CopyPairFolder-mac-arm64`](./CopyPairFolder-mac-arm64) |

---

## Windows 安装

1. 下载 `CopyPairFolder.exe`
2. 双击安装  
   - 先出现 **「正在安装，请稍后…」**（带加载动画）  
   - 完成后弹出 **一个**「安装成功」说明窗口，点「确定」关闭  
3. **关闭并重新打开**资源管理器窗口

安装目录：`%LOCALAPPDATA%\CopyPairFolder\`

> 说明：安装过程不会再弹出一串黑色控制台窗口。

### Windows 11

若默认精简右键看不到菜单项：

- 点 **「显示更多选项」**，或  
- 按住 **Shift** 再右键  

### 卸载

```bat
%LOCALAPPDATA%\CopyPairFolder\CopyPairFolder.exe /uninstall
```

卸载同样只显示「正在卸载」→ 完成提示，点确定关闭。

---

## macOS 安装（Apple Silicon）

1. 下载 `CopyPairFolder-mac-arm64`
2. 首次打开若提示无法验证：系统设置 → 隐私与安全性 → **仍要打开**
3. 双击运行完成安装（写入 Finder「快速操作 / 服务」）
4. 在 Finder 选中图片 → 右键 → **快速操作** → **复制并新建文件夹** / **剪切并新建文件夹**

若看不到该项：系统设置 → 键盘 → 键盘快捷键 → **服务**，勾选上述两项。

### 卸载

```bash
~/Library/Application\ Support/CopyPairFolder/CopyPairFolder --uninstall
```

---

## 从源码构建

```bash
# Windows
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 \
  go build -ldflags="-H windowsgui -s -w" -o CopyPairFolder.exe .

# macOS Apple Silicon（M3 / M4 / M5）
GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 \
  go build -ldflags="-s -w" -o CopyPairFolder-mac-arm64 .
```

---

## 常见问题

**Q：Win11 右键没有这项？**  
A：用「显示更多选项」/ Shift+右键；确认已用最新 exe **重新安装**，然后重开资源管理器。

**Q：Mac 右键没有快速操作？**  
A：到「系统设置 → 键盘 → 键盘快捷键 → 服务」勾选；或看右键菜单里的「服务」。

**Q：360 拦截？**  
A：这是对未代码签名小工具的常见误报。加入信任区 / 允许本次即可。本工具仅本地读写，无联网。

**Q：选了很多文件却只弹一次？**  
A：正常。多选时系统可能按文件多次启动程序，工具会合并为一次操作。

**Q：剪切会不会丢文件？**  
A：先全部复制进新文件夹成功后，再删除原位置文件。若删除个别失败，会提示，新文件夹里的副本仍保留。

**Q：安装时黑窗口狂闪？**  
A：旧版用 `reg.exe` 写菜单会导致控制台闪烁。请使用本仓库最新版：安装改为静默写注册表，只保留「安装中 / 安装成功」两个窗口。

---

## License

按现状提供，可自用与修改。
