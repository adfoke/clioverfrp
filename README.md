# ClioverFRP

用 FRP 把内网机器上的 `ragent` 暴露出来，再用本地 `lagent` 远程执行命令、列目录、上传和下载文件。

支持：
- `exec`：远程执行命令，实时输出
- `ls`：远程列目录
- `push`：上传，支持断点续传
- `pull`：下载，支持断点续传
- `task`：批量任务，读取 `jsonl`
- `cleanup`：清理本地断点文件
- 一个 `lagent` 管多个 `ragent`

## 结构

- 公网服务器：运行 `frps`
- 内网目标机器：运行 `ragent`，也可以由 `ragent` 顺手拉起 `frpc`
- `lagent`：本地 CLI
- 通信：WebSocket over FRP TCP

## 编译

要求：Go 1.26+

```bash
make build
```

生成：

- `bin/lagent`
- `bin/ragent`

## 安装 lagent

一键安装：

```bash
make install-lagent
```

默认会做两件事：

- 安装 `lagent` 到 `~/.local/bin/lagent`
- 如果 `~/.config/clioverfrp/config.yaml` 不存在，就写入一份示例配置

装完后：

```bash
lagent --help
lagent --list
```

## 统一配置

默认读当前目录的 `config.yaml`。没有就继续找：

- `config.yml`
- `~/.config/clioverfrp/config.yaml`
- `~/.config/clioverfrp/config.yml`

也可以手动指定：

```bash
export CLIOVERFRP_CONFIG=/path/to/config.yaml
```

项目根目录已经带了一个 `config.yaml` 示例，改这个文件就行。

多目标示例：

```yaml
agents:
  default: "dev"
  dev:
    ws_url: "ws://YOUR_FRPS_IP:60001/ws"
    token: "dev-token"
  prod:
    ws_url: "ws://YOUR_FRPS_IP:60002/ws"
    token: "prod-token"
```

## 打包远端目录

如果你想直接发一包给对方，先改好 `config.yaml`，再执行：

```bash
make bundle
```

产物目录：

```text
dist/ragent-bundle/
  ragent
  frpc
  frpc.toml
  start.sh
  README.txt
```

对方机器通常只要执行：

```bash
cd ragent-bundle
./start.sh
```

## FRP 配置

### frps 服务器

`frps.toml` 示例：

```toml
bindPort = 7000

auth.method = "token"
auth.token = "replace-with-your-frp-token"

webServer.addr = "0.0.0.0"
webServer.port = 7500
webServer.user = "admin"
webServer.password = "replace-with-dashboard-password"
```

启动：

```bash
./frps -c ./frps.toml
```

`auth.token` 建议用随机字符串。可直接生成：

```bash
openssl rand -hex 32
```

输出是 64 位十六进制字符串，可以直接填到 `auth.token`。

## 部署

### 远端机器

现在只推荐一种方式：在内网目标机器执行一个 `ragent` 命令，由它顺手拉起 `frpc`。

前提：

- 内网目标机器上已经有 `frpc` 可执行文件，或者你直接发 `bundle`
- `frps` 已经在公网服务器上启动

按 `config.yaml` 启动：

```bash
./ragent
```

如果要临时覆盖，命令行参数优先。

`agent.token` 也建议单独生成，不要和 FRP 的 `auth.token` 复用：

```bash
openssl rand -hex 32
```

如果你已经打过 `bundle`，更简单：

```bash
cd ragent-bundle
./start.sh
```

如果你已经有现成的 `frpc.toml`，也可以：

```bash
./ragent --frpc-config ./frpc.toml
```

后台运行：

```bash
nohup ./ragent > ragent.log 2>&1 &
```

默认监听地址：

```text
127.0.0.1:9000
```

也可以指定：

```bash
./ragent --listen 127.0.0.1:9000 --token "replace-with-your-agent-token"
```

### 本地机器

默认直接读同一个 `config.yaml`，本地通常不用再配别的文件。

如果要临时覆盖，也可以用环境变量：

```bash
export CLIOVERFRP_WS_URL="ws://YOUR_FRPS_IP:60001/ws"
export CLIOVERFRP_TOKEN="replace-with-your-agent-token"
export CLIOVERFRP_JSON=true
export CLIOVERFRP_QUIET=true
```

如果配置了多个目标，可以指定：

```bash
./lagent --list
./lagent --agent dev exec "hostname"
./lagent --agent prod ls /tmp
```

## 用法

### 基本命令

```bash
./lagent exec "whoami && uptime"
./lagent ls /tmp
./lagent push ./local.txt /tmp/remote.txt
./lagent pull /tmp/remote.txt ./download.txt
./lagent cleanup
```

### 常用参数

- `--list`：列出当前配置里的 ragent 目标
- `--agent`：选择目标 ragent
- `--json`：输出 JSON
- `--quiet`：减少非必要输出
- `--force`：覆盖已有文件或忽略旧断点
- `--skip-existing`：目标已存在时跳过

### 批量任务

```bash
./lagent task --file tasks.jsonl --json --quiet
```

`tasks.jsonl`：

```json
{"type":"exec","cmd":"uptime"}
{"type":"ls","path":"/tmp"}
{"type":"exec","agent":"prod","cmd":"hostname"}
{"type":"push","local":"./a.txt","remote":"/tmp/a.txt"}
{"type":"pull","remote":"/etc/hosts","local":"./hosts"}
```

执行后会生成 `task_report.json`。

规则：

- task 行里有 `agent` 时，用这一行自己的目标
- task 行里没有 `agent` 时，用 `--agent`
- 两者都没有时，用 `agents.default`

## JSON 输出

开启 `--json` 后，结果字段固定为：

- `success`
- `type`
- `output`
- `error`
- `duration_ms`
- `exit_code`

示例：

```bash
./lagent exec "df -h" --json --quiet
```

## 退出码

- `0`：成功
- `1`：连接或认证失败
- `2`：传输失败
- `3`：远程命令失败

## 断点续传

- `push` 和 `pull` 都支持续传
- 临时文件后缀默认是 `.clioverfrp.tmp`
- 元数据文件后缀是 `.meta.json`
- 重试同一条命令会自动续传
- `--force` 会丢弃旧断点，重新传输

## 安全

- `CLIOVERFRP_TOKEN` 和 FRP token 分开设置，不要混用
- 两个 token 都用长随机字符串
- `ragent` 不要用 root 跑
- FRP 端口只开放必要端口

## 最短流程

1. 在公网机器启动 `frps`
2. 改好 `config.yaml`
3. 在内网目标机器执行 `./ragent`
4. 在本地机器执行 `./lagent exec "hostname"`
