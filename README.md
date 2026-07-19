# 复制并新建文件夹（CopyPairFolder / exejpg）

Windows 10 / 11 资源管理器右键工具：选中图片后，一键把**同名且同时具备 JPG 与 CR3** 的配对文件复制到当前目录下的新文件夹，并定位到该文件夹方便重命名。

仓库：https://github.com/zxs-ai/exejpg

---

## 功能说明

| 项目 | 说明 |
|------|------|
| 右键菜单名 | **复制并新建文件夹** |
| 系统 | **Windows 10 / 11（64 位）** |
| 配对规则 | 同名文件同时存在 `.jpg` / `.jpeg` **和** `.cr3` 才复制 |
| 扩展名大小写 | 支持 `.jpg` / `.JPG` / `.jpeg` / `.JPEG` / `.cr3` / `.CR3` |
| 单格式文件 | 只有 JPG 或只有 CR3 → **不复制** |
| 新文件夹位置 | 选中文件所在的**当前目录** |
| 多选 | 支持多选；程序会合并选中项并只处理一次 |
| 进度提示 | 处理时弹出进度窗口 |
| 完成后 | 自动关闭进度窗，打开并选中新文件夹 |

---

## 安装

1. 下载 [`CopyPairFolder.exe`](./CopyPairFolder.exe)
2. 双击安装（会写入当前用户右键菜单）
3. **关闭并重新打开**资源管理器窗口（或注销一次）

### Windows 11 注意

Win11 默认是精简右键。若看不到「复制并新建文件夹」：

- 点右键菜单底部的 **「显示更多选项」**，或  
- 按住 **Shift** 再右键  

即可看到完整菜单。

---

## 卸载

```bat
%LOCALAPPDATA%\CopyPairFolder\CopyPairFolder.exe /uninstall
```

---

## 从源码构建

```bash
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 \
  go build -ldflags="-H windowsgui -s -w" -o CopyPairFolder.exe .
```

---

## 常见问题

**Q：Win11 右键没有这项？**  
A：请用「显示更多选项」/ Shift+右键；并确认已用最新 exe **重新安装**，然后重开资源管理器。

**Q：360 拦截？**  
A：加入信任区即可。本工具仅本地读写，无联网。

---

## License

按现状提供，可自用与修改。
