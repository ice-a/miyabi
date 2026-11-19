# Miyabi

配合本地媒体目录的一站式 Jav & Emby 管理平台。

Miyabi 直接扫描本机或 NAS 上已挂载的媒体目录，负责刮削、订阅与 STRM 导出，也可作为底层服务对 Emby 提供支持。

> [!NOTE]
> 请注意，JavDB 目前收紧了 API 调用策略，可能会导致账号被封禁。在解决这个问题前，请不要使用本项目。

## 项目预览

![媒体库](./screenshots/ScreenShot_2026-09-19_131853_357.png)
![发现](./screenshots/ScreenShot_2026-09-19_132213_340.png)
![搜索](./screenshots/ScreenShot_2026-09-19_132227_063.png)
![设置](./screenshots/ScreenShot_2026-09-19_132249_331.png)

## 功能特性

- **本地媒体目录扫描**：扫描本机磁盘或已挂载 NAS（NFS/SMB 挂载点）上的影片、NFO 与字幕，自动识别番号并建立媒体库索引。
- **元数据刮削**：聚合 FANZA、JavDB、JavBus、R18、MissAV 等多个元数据来源，含封面、预览图、演员与标签；演员头像由 Griends 补全。
- **磁力订阅**：追踪影片与演员新作，命中磁力后记录在订阅页，复制磁力链交给你的下载工具，落盘后触发扫描即可入库。
- **STRM 导出**：为 Emby 生成 `.strm` 播放文件与 NFO 元数据；播放请求由 Miyabi 直接读取媒体文件并以 Range 分段转发，支持拖动进度。
- **任务系统**：扫描、刮削、订阅批量任务统一排队执行，进度通过 SSE 实时推送。

## 本地启动（源码运行）

适合二次开发或在无法使用 Docker 的机器上直接运行。前置要求：

- Go 1.27+
- Node.js 20+ 与 pnpm 9+（仅前端需要）
- 一个已存在的媒体目录（含按番号命名的影片文件或 `.strm`，可选 NFO/海报/字幕）

### 1. 构建前端

```bash
cd web
pnpm install --frozen-lockfile
pnpm build
```

构建产物输出到 `web/dist`。**必须先完成这一步**：不带 `dev` 标签的 Go 构建会把 `web/dist` 嵌入二进制（`embed.go` 的 `//go:embed web/dist`），目录缺失会直接编译失败。

### 2. 编译并运行后端

在仓库根目录执行：

```bash
go build -o miyabi ./cmd
./miyabi
```

或直接运行：

```bash
go run ./cmd
```

默认监听 `:8080`、数据目录为 `./data`，访问 `http://127.0.0.1:8080` 即可进入 Web 界面。建议至少配置访问密码：

```bash
MIYABI_ACCESS_PASSWORD='change-this-password' ./miyabi
```

全部环境变量见下方「启动配置」。

### 3. 挂载媒体目录并扫描

1. 打开「设置」→「设置来源」页顶部的「媒体目录」，填写媒体目录的**绝对路径**（如 `/mnt/media` 或 Windows 下的 `D:\Media`），保存后 Miyabi 会校验目录存在并持久化挂载。
2. 在同一页的「存储来源」中添加一个本地目录存储源（指向同一目录），保存后会自动入队首次扫描。
3. 扫描进度可在「任务」页实时查看。扫描完成后即可在「媒体库」浏览，刮削任务会自动排队。

### 开发模式（前后端分离，热更新）

前端改动频繁时，用 Vite 开发服务器替代静态嵌入：

```bash
# 终端 1：后端以 dev 标签运行（不嵌入前端，跳过 web/dist 依赖）
go run -tags dev ./cmd

# 终端 2：前端开发服务器（http://127.0.0.1:5173，/api 自动代理到 127.0.0.1:8080）
cd web && pnpm dev
```

访问 `http://127.0.0.1:5173` 进行开发；后端日志为 JSON 格式输出到 stdout。

### 本地启动常见问题

| 现象 | 原因与处理 |
| --- | --- |
| `go build` 报 `pattern web/dist: no matching files found` | 未构建前端，先执行 `cd web && pnpm install && pnpm build`；或改用 `go build -tags dev` |
| 打开页面显示空白或 404 | 未构建前端就带了 `-tags dev` 运行（dev 标签不嵌入 UI），先完成 `pnpm build` 再不带标签运行 |
| 播放报「视频文件已移动或删除」 | 影片文件不在当前挂载的媒体目录内，检查「媒体目录」路径与文件实际位置是否一致 |
| 挂载媒体目录报「目录不存在或已被移除」 | 填写的路径在运行 Miyabi 的进程视角下不可见；Docker 场景需确认 `-v` 挂载与容器内路径一致 |

## Docker 部署

> \[!TIP]
> 您可以借助 AI 完成项目的部署，如果要对公网访问，请注意网络安全。

公开镜像：

```text
ghcr.io/ppxb/miyabi
```

### 1. 使用 Docker CLI 运行

请将 `/path/to/data` 替换为主机上的数据持久化目录，并将 `change-this-password` 替换为自定义密码：

```bash
docker run -d \
  --name miyabi \
  --restart unless-stopped \
  --security-opt no-new-privileges:true \
  -p 8080:8080 \
  -v /path/to/data:/app/data \
  -v /path/to/media:/media \
  -e MIYABI_ACCESS_PASSWORD='change-this-password' \
  -e MIYABI_PUBLIC_URL='http://<宿主机IP>:8080' \
  ghcr.io/ppxb/miyabi:latest
```

> [!NOTE]
> 如果配合 Emby 使用，生成的 STRM 播放直链与元数据默认输出在 `/app/data/emby`（对应宿主机目录 `/path/to/data/emby`）。您可将该目录挂载至 Emby 容器作为媒体库。

### 2. 使用 Docker Compose（推荐）

创建 `docker-compose.yml` 文件：

```yaml
services:
  miyabi:
    image: ghcr.io/ppxb/miyabi:latest
    container_name: miyabi
    restart: unless-stopped
    security_opt:
      - no-new-privileges:true
    ports:
      # 使用反向代理时可改为 "127.0.0.1:8080:8080"，仅监听宿主机回环地址
      - "8080:8080"
    volumes:
      - /path/to/data:/app/data
      - /path/to/media:/media
    environment:
      - MIYABI_ACCESS_PASSWORD=change-this-password
      - MIYABI_PUBLIC_URL=http://<宿主机IP>:8080
      # 可选：直接通过环境变量预设 Emby 集成（也可启动后在 Web 设置页中配置）
      # - MIYABI_EMBY_ENABLED=true
      # - MIYABI_EMBY_SERVER_URL=http://<Emby_IP>:8096
      # - MIYABI_EMBY_API_KEY=your-emby-api-key
      # - MIYABI_EMBY_MEDIA_PATH=/media
```

执行启动：

```bash
docker compose up -d
```

启动后访问 `http://<服务器IP>:8080`，随后按「本地启动」第 3 步挂载媒体目录并触发扫描。

查看日志：

```bash
docker logs -f miyabi
```

### 升级镜像

升级时先拉取新镜像，再重新创建容器：

```bash
docker pull ghcr.io/ppxb/miyabi:latest
docker stop miyabi
docker rm miyabi
```

然后重新执行 `docker run` 命令，沿用原访问密码和数据目录。

使用 Docker Compose 时，在 Compose 目录下执行：

```bash
docker compose pull && docker compose up -d
```

## 启动配置

启动配置统一使用环境变量，未设置时采用默认值。监听地址与数据目录显式设为空时会报错。

通过 `MIYABI_ACCESS_PASSWORD` 配置访问密码。未配置密码或将密码设为空时关闭门禁。**强烈建议您开启门禁**。

| 环境变量                 | 用途                                                  | 默认值                                        |
| ------------------------ | ----------------------------------------------------- | --------------------------------------------- |
| `MIYABI_ACCESS_PASSWORD` | Web 入口访问密码                                      | 空，关闭门禁（强烈建议配置）                  |
| `MIYABI_JWT_SECRET`      | 登录 JWT 的签名与校验密钥                              | 空，开启门禁时由访问密码派生固定密钥           |
| `MIYABI_TRUSTED_PROXIES` | 可信代理 IP 或 CIDR，用于识别客户端 IP，多个用逗号分隔 | 空，不信任任何代理                            |
| `MIYABI_LOG_LEVEL`       | 日志级别，可选 `debug`、`info`、`warn`、`error`         | `info`                                        |
| `MIYABI_PUBLIC_URL`      | 外部访问 Miyabi 的根地址，用于生成 STRM 播放直链      | 默认根据监听端口推导                          |
| `MIYABI_LISTEN`          | HTTP 监听地址                                         | `:8080`                                       |
| `MIYABI_DATA_DIR`        | SQLite 与图片缓存目录                                 | 容器内为 `/app/data`，二进制为 `./data`       |
| `MIYABI_EMBY_DIR`        | STRM 与媒体导出目录                                   | `$MIYABI_DATA_DIR/emby`                       |
| `MIYABI_EMBY_ENABLED`    | 是否启用 Emby 集成通知刷新                            | 配置了服务器或密钥时自动为 `true`             |
| `MIYABI_EMBY_SERVER_URL` | Emby 服务器访问地址（如 `http://192.168.1.100:8096`） | 空，可在 Web 界面动态配置                     |
| `MIYABI_EMBY_API_KEY`    | Emby API 密钥（在 Emby「高级」→「API 密钥」中生成）   | 空，可在 Web 界面动态配置                     |
| `MIYABI_EMBY_MEDIA_PATH` | Emby 容器内挂载的媒体库路径（如 `/media`）            | 空（默认使用本地路径），可在 Web 界面动态配置 |
| `MIYABI_EMBY_SYNC_ACTORS`| 是否同步演员头像到 Emby                               | 空，默认关闭                                  |
| `MIYABI_STRM_TOKEN`      | STRM 播放直链访问鉴权 Token                           | 空，未启用 Token 鉴权                         |

部署在 nginx 等反向代理后面时，需将反向代理连接 Miyabi 时使用的 IP 填入 `MIYABI_TRUSTED_PROXIES`，以便正确识别客户端 IP。

> [!IMPORTANT]
> 存储源现在只有本地目录一种。请在「设置」→「设置来源」中挂载媒体目录（本机磁盘或已挂载到容器的 NAS 路径），挂载容器时须把这个目录一并 `-v` 进去，并保证容器内路径与设置页填写的路径一致。旧版本遗留的 115 存储源会被标记为已下线，重新添加一个指向本机/NAS 目录的存储源即可恢复扫描与播放。

> [!NOTE]
> 刮削来源（FANZA、JavDB、JavBus、MissAV 等）与封面、预览图均从境外站点抓取，国内网络环境需在「设置」→「网络」中配置 HTTP 代理，否则分类浏览与图片加载会失败或超时。MissAV 可在「设置」→「影片刮削」中单独开关。

> [!TIP]
> Emby 相关配置（服务器地址、API Key、媒体库路径等）除通过环境变量在容器初始化时配置外，也可以在服务启动后随时通过 Web 界面「设置」→「Emby」中进行可视化配置与连通性测试。

## NSFW 警告

本软件可能存在裸露、暴力、色情或冒犯等不适宜公众场合的内容，请勿在公共场合使用本软件，避免不必要的纷争。

## 特别鸣谢

感谢 [莫愁](https://github.com/zk020106) 的 token，感谢 [老罗](https://github.com/luoqiz) 早期版本中的账号支持。

## 致谢

本项目参考了以下项目的部分实现，在此表示衷心的感谢！

- [javdb-cli](https://github.com/FlanChanXwO/javdb-cli)

同时感谢社区 [LinuxDO](https://linux.do) 的帮助。

## 联系我们

[Telegram](https://t.me/miyabi_cn)

## 免责声明

本项目仅供学习、研究和技术交流使用。项目作者与任何第三方服务、原始应用或内容提供方无关。
使用者应自行遵守当地法律法规以及相关服务条款。因使用本项目产生的任何法律、版权、账号、数据或财务风险均由使用者自行承担。

## License

遵循 [GNU GPL v3](./LICENSE) 协议。
