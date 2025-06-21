# 📧 邮件附件下载器

> 基于 Go 1.24 + Wails + Vue3 + Naive UI 的现代化邮件附件自动下载工具

## 🌟 功能特性

### 🔧 核心功能
- **多邮箱支持** - 同时管理多个邮箱账户，支持QQ邮箱、163邮箱等主流IMAP服务
- **智能PDF检测** - 自动识别邮件附件和HTML链接中的PDF文件
- **断点续传** - 支持大文件的断点续传，网络中断后自动恢复下载
- **多线程下载** - 高性能并发下载，可配置最大并发数
- **文件去重** - 智能避免重复下载相同文件

### 💎 用户体验
- **现代化UI** - 基于Naive UI的简约美观界面
- **实时进度** - 实时显示下载进度和速度
- **系统托盘** - 支持最小化到系统托盘，后台运行
- **智能通知** - 新邮件和下载完成的系统通知
- **主题切换** - 支持明暗主题自动切换

### ⚡ 性能优化
- **内存优化** - 流式处理大文件，避免内存溢出
- **数据库缓存** - SQLite数据库存储任务状态和统计信息
- **连接复用** - IMAP连接池减少连接开销
- **编码兼容** - 完美支持中文文件名和各种字符编码

## 🏗️ 项目架构

```
email-pdf-downloader/
├── backend/                    # Go后端代码
│   ├── models/                # 数据模型
│   │   └── models.go         # 邮箱、任务、配置等模型定义
│   ├── database/              # 数据库操作
│   │   └── database.go       # SQLite数据库操作封装
│   ├── services/              # 业务服务
│   │   ├── email_service.go  # 邮件处理服务（IMAP连接、邮件解析）
│   │   ├── download_service.go # 下载服务（多线程、断点续传）
│   │   └── tray_service.go   # 系统托盘服务
│   ├── utils/                 # 工具函数
│   │   └── utils.go          # 文件处理、编码转换等工具
│   └── app.go                # 主应用结构体和API接口
├── frontend/                  # Vue3前端代码
│   ├── src/
│   │   ├── components/       # Vue组件
│   │   ├── stores/           # Pinia状态管理
│   │   ├── router/           # 路由配置
│   │   ├── utils/            # 前端工具函数
│   │   └── App.vue           # 根组件
│   ├── package.json          # 依赖配置
│   └── vite.config.ts        # 构建配置
├── go.mod                     # Go模块依赖
├── wails.json                 # Wails配置
└── main.go                    # 程序入口
```

## 🚀 技术栈

### 后端技术
- **Go 1.24** - 高性能的后端语言
- **Wails v2** - 现代化的桌面应用框架
- **GORM + SQLite** - 数据持久化
- **go-imap** - IMAP邮件协议处理
- **goquery** - HTML解析和链接提取
- **systray** - 系统托盘集成

### 前端技术
- **Vue 3** - 渐进式JavaScript框架
- **TypeScript** - 类型安全的JavaScript
- **Naive UI** - 优雅的Vue 3组件库
- **Pinia** - 新一代状态管理库
- **Vue Router** - 路由管理
- **Echarts** - 数据可视化图表

## 📦 安装使用

### 开发环境要求
- Go 1.24+
- Node.js 16+
- Wails CLI v2

### 构建步骤

1. **克隆项目**
```bash
git clone https://github.com/your-repo/email-pdf-downloader.git
cd email-pdf-downloader
```

2. **安装前端依赖**
```bash
cd frontend
npm install
```

3. **构建应用**
```bash
wails build
```

4. **运行开发模式**
```bash
wails dev
```

## ⚙️ 配置说明

### 邮箱配置
- **IMAP服务器设置** - 支持自定义IMAP服务器和端口
- **SSL/TLS加密** - 默认启用安全连接
- **授权码认证** - 建议使用邮箱授权码而非登录密码

### 下载配置
- **下载路径** - 可自定义PDF文件保存目录
- **并发数控制** - 可配置同时下载的最大文件数（默认3个）
- **检查间隔** - 可设置自动检查邮件的时间间隔

## 🎯 使用场景

- **论文收集** - 自动下载学术期刊发送的PDF论文
- **发票管理** - 自动收集各类电子发票PDF文件
- **文档归档** - 批量下载邮件中的重要文档
- **资料整理** - 自动整理各种PDF格式的学习资料


---

⭐ 如果这个项目对您有帮助，请给它一个星标！ 