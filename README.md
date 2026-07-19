# 复制并新建文件夹（CopyPairFolder / exejpg）

Windows 10 资源管理器右键工具：选中图片后，一键把**同名且同时具备 JPG 与 CR3** 的配对文件复制到当前目录下的新文件夹，并定位到该文件夹方便重命名。

仓库：https://github.com/zxs-ai/exejpg

---

## 功能说明

| 项目 | 说明 |
|------|------|
| 右键菜单名 | **复制并新建文件夹** |
| 配对规则 | 同名文件同时存在 `.jpg` / `.jpeg` **和** `.cr3` 才复制 |
| 扩展名大小写 | 支持 `.jpg` / `.JPG` / `.jpeg` / `.JPEG` / `.cr3` / `.CR3` |
| 单格式文件 | 只有 JPG 或只有 CR3 → **不复制** |
| 新文件夹位置 | 选中文件所在的**当前目录** |
| 多选数量 | 无 15 / 100 张限制（COM 右键实现，可选 100、200 张及以上） |
| 进度提示 | 处理时弹出进度窗口：进度条 +「共 N 张，已复制 M 张」 |
| 完成后 | 自动关闭进度窗，打开并选中新文件夹，便于重命名 |

### 配对示例

假设当前目录有：

```
a12.JPG
a12.CR3
b01.JPG          ← 只有 JPG，不复制
c02.CR3          ← 只有 CR3，不复制
d03.jpeg
d03.cr3
```

选中后执行「复制并新建文件夹」时，新文件夹中只会有：

```
a12.JPG
a12.CR3
d03.jpeg
d03.cr3
```

若只选中了 `a12.JPG`，程序会在同目录查找配对的 `a12.CR3`（或 `.cr3`）；两边都存在则一起复制。

---

## 快速开始（用户）

### 安装

1. 下载本仓库中的 [`CopyPairFolder.exe`](./CopyPairFolder.exe)
2. 在 Windows 10 上双击运行
3. 出现「安装成功」提示即可
4. 打开资源管理器，选中图片 → 右键 → **复制并新建文件夹**

> 建议安装后关闭并重新打开资源管理器窗口，菜单刷新更可靠。

### 使用

1. 选中一张或多张图片（可混合选中有效/无效文件）
2. 右键点击 **复制并新建文件夹**
3. 等待进度窗口显示「共 N 张，已复制 M 张」
4. 完成后新文件夹会出现在当前目录（名称形如 `配对导出_20260719_153045`）
5. 资源管理器会定位到该文件夹，可直接改名

### 卸载

在「运行」或命令提示符中执行：

```bat
%LOCALAPPDATA%\CopyPairFolder\CopyPairFolder.exe /uninstall
```

或直接打开：

```
C:\Users\<你的用户名>\AppData\Local\CopyPairFolder\CopyPairFolder.exe /uninstall
```

---

## 行为与限制（设计说明）

- **只写当前用户注册表（HKCU）**，一般不需要管理员权限
- **无网络请求、无进程注入**，尽量降低杀软误报概率（不能保证 360 等一定不误报；若误报可加白名单）
- 多选时资源管理器可能多次拉起进程：程序用会话锁保证**只建一个文件夹、只弹一次结果**
- 「共 N 张」指**实际会复制的文件数**（有效配对），不是选中但因缺配对而被跳过的数量

---

## 开发者：从源码构建

### 环境

- Go 1.21+（推荐较新版本）
- 目标平台：Windows amd64

### 源码结构

```
.
├── main.go               # 安装/卸载、配对复制主逻辑
├── com_windows.go        # COM IExecuteCommand 右键（突破多选数量限制）
├── progress_windows.go   # 进度条窗口（PowerShell WPF）
├── go.mod
├── go.sum
├── CopyPairFolder.exe    # 预编译发布包（Windows）
└── README.md
```

### 交叉编译（在 macOS / Linux 上）

```bash
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 \
  go build -ldflags="-H windowsgui -s -w" -o CopyPairFolder.exe .
```

### 在 Windows 本机编译

```bat
go build -ldflags="-H windowsgui -s -w" -o CopyPairFolder.exe .
```

### 命令行参数

| 参数 | 作用 |
|------|------|
| （无参数 / 双击） | 安装右键菜单 |
| `/install` | 安装 |
| `/uninstall` | 卸载 |
| `/help` | 帮助说明 |
| `/Embedding` | COM 本地服务器（由系统调用，用户无需手动使用） |
| `/process-list <文件>` | 从列表文件处理选中项（内部使用） |

安装后程序会复制到：

```
%LOCALAPPDATA%\CopyPairFolder\CopyPairFolder.exe
```

并注册：

- `HKCU\Software\Classes\*\shell\CopyPairFolder`
- `HKCU\Software\Classes\CLSID\{6C1D6A92-8E2B-4B5C-9F17-1D6A3170C8E4}`（DelegateExecute）

---

## 常见问题

**Q：选中超过 15 张后右键没有菜单？**  
A：请使用本仓库最新版（COM 实现）。旧版传统 `command` + `Document` 会被系统限制在 15 个。安装新版后重开资源管理器再试。

**Q：进度窗口不关闭？**  
A：请重新双击最新 `CopyPairFolder.exe` 覆盖安装（会更新进度脚本）。

**Q：提示未找到配对？**  
A：确认同名的 JPG/JPEG 与 CR3 都在同一文件夹；扩展名大小写均可。

**Q：360 报毒？**  
A：本工具会写注册表、读写本地文件，部分杀软可能误报。可加入信任区后使用。

---

## License

本仓库代码按现状提供，可按需自用与修改。若你分发修改版，建议保留功能说明以免用户误解配对规则。
