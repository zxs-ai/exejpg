# 复制并新建文件夹（CopyPairFolder / exejpg）

资源管理器 / Finder 右键工具：选中图片后，一键把**同名且同时具备 JPG 与 CR3** 的配对文件复制到当前目录下的新文件夹，并定位该文件夹方便重命名。

仓库：https://github.com/zxs-ai/exejpg  
关联审片 App：[图像管理工具 ImgMan](https://github.com/zxs-ai/ImgMan)

---

## 功能说明

| 项目 | 说明 |
|------|------|
| 菜单名 | **复制并新建文件夹** |
| 系统 | **Windows 10 / 11（x64）**、**macOS Apple Silicon（M3 / M4 / M5，arm64）** |
| 配对规则 | 同名文件同时存在 `.jpg` / `.jpeg` **和** `.cr3` 才复制 |
| 扩展名大小写 | 支持 `.jpg` / `.JPG` / `.jpeg` / `.JPEG` / `.cr3` / `.CR3` |
| 单格式文件 | 只有 JPG 或只有 CR3 → **不复制** |
| 新文件夹位置 | 选中文件所在的**当前目录** |
| 多选 | 支持；合并选中项并只处理一次（防多开） |
| 进度提示 | Windows 有进度窗；macOS 有通知 + 完成对话框 |
| 完成后 | 打开并选中新文件夹（便于 F2 / 回车改名） |
| 右键位置 | Windows 注册 `Position=Top`，尽量靠前显示 |

---

## 下载

| 平台 | 文件 |
|------|------|
| Windows | [`CopyPairFolder.exe`](./CopyPairFolder.exe) |
| macOS Apple Silicon | [`CopyPairFolder-mac-arm64`](./CopyPairFolder-mac-arm64) |

---

## Windows 安装

1. 下载 `CopyPairFolder.exe`
2. 双击安装（写入当前用户右键菜单）
3. **关闭并重新打开**资源管理器窗口

### Windows 11

若默认精简右键看不到「复制并新建文件夹」：

- 点 **「显示更多选项」**，或  
- 按住 **Shift** 再右键  

### 卸载

```bat
%LOCALAPPDATA%\CopyPairFolder\CopyPairFolder.exe /uninstall
```

---

## macOS 安装（Apple Silicon）

1. 下载 `CopyPairFolder-mac-arm64`
2. 首次打开若提示无法验证：系统设置 → 隐私与安全性 → **仍要打开**
3. 双击运行完成安装（写入 Finder「快速操作 / 服务」）
4. 在 Finder 选中图片 → 右键 → **快速操作** → **复制并新建文件夹**

若看不到该项：系统设置 → 键盘 → 键盘快捷键 → **服务**，勾选「复制并新建文件夹」。

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
A：正常。多选时系统可能按文件多次启动程序，工具会合并为一次复制。

---

## License

按现状提供，可自用与修改。
