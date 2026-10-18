# Picpac 部署操作手册

仓库：`juemiaoluoji/picpac`。环境：腾讯云 Ubuntu 服务器上的自建 **k3s**，部署不调用托管 TKE 接口。本文只展示配置名称，不记录真实凭证；Sites 网页不随本次修改更新。

部署改动在[草稿 PR #1](https://github.com/juemiaoluoji/picpac/pull/1)。合并后，`main` 的推送才会使用本手册的自动流程；未合并时可手动运行 `codex/container-image` 分支。

## 1. 合入代码，自动构建镜像

每次合入或直接推送代码到 `main`，GitHub 自动运行 Actions。测试和镜像检查通过、发布配置齐全后，会继续推送镜像并部署。PR 检查只验证，不发布、不部署。

| 项目 | 说明 |
| --- | --- |
| 流水线 | `.github/workflows/container-image.yml`，页面名称 **Container image** |
| 构建方式 | GitHub 标准 `ubuntu-latest` Runner 构建；个人电脑无需 Docker，服务器无需安装自托管 Runner |
| Dockerfile | 仓库根目录 `Dockerfile`；构建阶段使用 `golang:1.25-bookworm`，运行阶段使用 `scratch` |
| 镜像内容 | Go 程序、`category.json`、CA 证书，以非 root 用户运行；不打包运行配置和密钥 |
| 自动顺序 | 测试 → go vet → 构建并检查镜像 → 推送腾讯云仓库 → SSH 部署 k3s |
| 免费额度 | 当前仓库公开，标准托管 Runner 免费；GitHub Free **私有仓库每月 2,000 分钟**，约 33 小时 20 分钟，按账号共享，不是 2,000 小时 |

免费规则参见 [GitHub Actions 计费说明](https://docs.github.com/en/billing/concepts/product-billing/github-actions)。工作流不上传 Docker 构建记录 Artifact，Go 缓存也已关闭；现有服务器和业务服务的费用另算。

## 2. 手动推送镜像、部署或回退

在 **Actions → Container image → Run workflow** 中选择分支和参数，然后点击运行。

| 操作 | 参数 / 入口 |
| --- | --- |
| 只测试和构建 | `publish=false`；不会把镜像保存到腾讯云 |
| 构建并推送，不部署 | `publish=true`、`deploy=false` |
| 构建、推送并部署 | `publish=true`、`deploy=true`；架构通常选 `linux/amd64` |
| 只部署已有镜像 / 回退 | **Deploy to k3s → Run workflow**，填写完整镜像地址，如 `ccr.ccs.tencentyun.com/picpac/server:30cded3`；不重新构建 |

也可在电脑 PowerShell 中执行：

```powershell
gh workflow run container-image.yml --repo juemiaoluoji/picpac --ref main -f publish=true -f deploy=true -f platform=linux/amd64
gh workflow run deploy-k3s.yml --repo juemiaoluoji/picpac --ref main -f image=ccr.ccs.tencentyun.com/picpac/server:30cded3 -f platform=linux/amd64
gh run list --repo juemiaoluoji/picpac --limit 5
```

工作流未合并 `main` 时，将 `--ref main` 换为 `--ref codex/container-image`；页面没有手动按钮时可用 CLI。修改 Secret 或 Variable 不会自动触发部署，需要重新运行。

## 3. 镜像仓库、tag 与推送凭证

| 项目 | 说明 |
| --- | --- |
| 仓库 | 腾讯云 TCR 个人版：`ccr.ccs.tencentyun.com/picpac/server` |
| tag | 代码提交 SHA 的前 **7 位**，例如 `30cded3`；完整镜像地址为 `ccr.ccs.tencentyun.com/picpac/server:30cded3` |
| 地址配置 | GitHub Variable `TCR_IMAGE`，不带协议和 tag |
| 登录凭证 | GitHub Secrets `TCR_USERNAME`、`TCR_PASSWORD`；从腾讯云 TCR 个人版的访问凭证 / 登录指引获取，不是服务器 SSH 密码 |
| 服务器拉取 | 部署脚本复用上述凭证，生成 k3s 命名空间 `picpac` 内的 `tcr-pull` Secret，供 Pod 拉取私有镜像 |
| 是否免费 | TCR 个人版限额免费；官方默认单仓库最多 100 个镜像版本，满额需清理旧 tag。无需购买企业版实例 |

参见腾讯云[个人版免费说明](https://cloud.tencent.com/document/product/1141/39278)和[配额说明](https://cloud.tencent.com/document/faq/1141/57780)。

## 4. GitHub 通过 SSH 更新 k3s

| 项目 | 说明 |
| --- | --- |
| 目标机器 | `SSH_HOST` 指定服务器；当前公网地址为 `43.142.86.222` |
| 登录账号 | `SSH_USER`，当前使用 `ubuntu` |
| 登录密钥 | `SSH_PRIVATE_KEY`：已绑定服务器公钥对应的完整 PEM 私钥，不是文件路径；证明登录者身份 |
| 主机校验 | `SSH_KNOWN_HOSTS`：事先确认的服务器 SSH 主机公钥记录；证明服务器身份，不是 PEM 私钥。目前仍保存在 Secret 中 |
| 网络与提权 | 默认 SSH TCP `22`，实例防火墙需允许 Runner 连接；账号必须能免密码运行 `sudo -n -i` |
| 部署文件 | `.github/workflows/deploy-k3s.yml` 调用 `deploy/deploy-k3s.sh`，Deployment 模板为 `deploy/picpac.yaml` |
| 首次部署 | 创建命名空间、配置与拉取凭证 Secret、Deployment 和 NodePort Service |
| 后续部署 | 更新镜像和配置；k3s 拉取镜像并滚动更新 Pod，流水线等待就绪。已有副本数、资源限制、其他容器及 Service 保留 |
| 公钥何时会变 | 重装系统、重建机器或重新生成 SSH 主机密钥时可能变化；换 IP / SSH 端口时也需更新记录。普通重启、更新业务镜像不会改变它 |

在电脑 PowerShell 查看已信任的主机记录：

```powershell
ssh-keygen -F 43.142.86.222
```

需要重新获取时，在腾讯云控制台登录服务器执行：

```bash
sudo awk '{print "43.142.86.222", $1, $2}' /etc/ssh/ssh_host_ed25519_key.pub
```

把完整一行写入 `SSH_KNOWN_HOSTS`。非 22 端口时第一列使用 `[服务器IP]:端口`。主机公钥可放普通 Variable，但迁移时必须同步修改工作流；不能只移动配置或删除当前 Secret。不要每次联网扫描后直接信任新公钥。

## 5. 生产配置与 Mongo

配置来源改为根目录 **`config.prod.json`**，本地开发仍使用原 `config.json`。生产模板中的非敏感占位符如 `${MONGO_SERVICE}` 从 Actions Variables 获取；凭证统一保留 `偷apikey的替我挡灾`，由 `deploy/render-config.py` 按字段注入独立 Secrets。工作流通过 `${{ vars.名称 }}` / `${{ secrets.名称 }}` 传入环境变量，使用 JSON 序列化生成配置，不把真实值提交到 Git。

| 配置项 | 配置来源 / 行为 |
| --- | --- |
| 普通生产配置 | 修改 `config.prod.json`，如端口、OSS endpoint、bucket、模型、CORS 和超时等；`env` 固定为 `prod` |
| Mongo 服务名 | Variable `MONGO_SERVICE`，填写 `服务名.命名空间.svc.cluster.local`；使用稳定 Service DNS，不用 Pod 名称或 Pod IP |
| Mongo 端口 | Variable `MONGO_PORT`：Service 内部端口，通常 `27017`，以实际 Service 配置为准；不是节点对外端口 `32017`，也不一定等于 Pod 的 targetPort |
| Mongo 用户名 / 数据库 | Variables `MONGO_USERNAME`、`MONGO_DATABASE`、`MONGO_AUTH_SOURCE`；用户名也是认证所需字段，不需要作为 Secret |
| Mongo 密码 | 独立 Secret `MONGO_PASSWORD`；脚本分别对用户名和密码 URL 编码后组装运行时 `mongo.uri`，应用配置结构不变 |
| 已有服务升级 | 未填写服务名、端口、用户名时可复用服务器旧配置；同一地址、端口和用户名未变时，未配置密码也可复用旧密码。不再读取 Mongo Pod 环境或通过 NodePort 查找 |
| 首次部署 / 更换 Mongo | 填写 `MONGO_SERVICE`、`MONGO_USERNAME`、`MONGO_PASSWORD`；内部端口默认 `27017`，数据库默认 `picpac`，认证库默认 `admin`，按实际修改。缺失必需项时部署报错，不修改 Mongo 本身 |
| JWT | Secret `JWT_SECRET` 可选；未配置时复用服务器旧密钥，否则首次在服务器生成并持久化。主动更换会影响已有登录令牌 |
| 外部服务 | OSS、短信、DeepSeek、DashScope 分别读取第 7 节列出的独立 Secret；未配置时相关调用不可用，不代表凭证有效 |
| 配置落地 | Actions 临时 JSON → 服务器 `/picpac/config/config.json`（权限 600）→ k3s Secret `picpac-config` → 容器 `/app/config/config.json`，镜像不包含配置 |

每次部署重新生成配置；长期改动应修改 prod 模板或 GitHub 配置，不要只改服务器文件。已有连接的副本集、TLS 等参数在地址和端口不变时保留；首次需要额外参数时，可在 prod 模板的 `mongo.options` 中填写。

在服务器查询 Mongo 的实际 Service 和内部端口：

```bash
sudo -i
k3s kubectl get svc -A
```

例如输出命名空间 `data`、服务名 `mongodb`、端口 `27017:32017/TCP`，则填 `MONGO_SERVICE=mongodb.data.svc.cluster.local`、`MONGO_PORT=27017`。这只是示例，需以你的实际输出为准。应用通过集群内地址访问，不需要开放 Mongo 公网端口。原 `MONGO_NODE_PORT` Variable 已不再被工作流读取，可删除。

## 6. 端口与访问方式

| 用途 | 端口 / 地址 | 说明 |
| --- | --- | --- |
| 应用容器监听 | `9090/TCP` | 来自 prod 配置 `server.port`；Dockerfile 的 EXPOSE 不会自动开放公网端口 |
| 应用 NodePort | `32090/TCP` | 首次创建时来自 `K3S_NODE_PORT`；已有 Service 不会自动改端口 |
| 外部接口 | `http://43.142.86.222:32090/api/v1/categories` | 当前使用 HTTP；实例防火墙须放行 TCP `32090` |
| SSH | 默认 `22/TCP` | GitHub Runner 登录和部署使用 |
| Mongo 内部连接 | Service DNS + `MONGO_PORT` | 使用 Service 的集群内端口 |
| Mongo 旧 NodePort | `32017/TCP` | 现有 Mongo 对外映射，应用内部连接不依赖它 |

在服务器检查服务：

```bash
sudo -i
k3s kubectl -n picpac get deployment,pods,svc
k3s kubectl -n picpac logs deployment/picpac --tail=50
curl http://127.0.0.1:32090/api/v1/categories
```

本机可访问、外网超时时，检查这台服务器**实例**的 TCP `32090` 防火墙；修改模板不会自动修改既有实例。Pod `1/1 Ready` 不等于公网规则已放行。

## 7. Secrets：说明、GitHub 页面与 CLI

Secret 用于密码、私钥和 API Key，保存后加密存储到 GitHub；页面和 CLI 不提供明文回读，只能更新。工作流中获授权的步骤可使用明文，不能打印到日志。以下“已配置”仅核对名称存在，值列不记录真实凭证。

| 名称 | 状态 | 说明 | 值 |
| --- | --- | --- | --- |
| `TCR_USERNAME` | 已配置，必需 | TCR 登录用户名 / 账号 ID | |
| `TCR_PASSWORD` | 已配置，必需 | TCR 访问密码；推送和 k3s 拉取共用 | |
| `SSH_HOST` | 已配置，必需 | 服务器地址；非敏感，当前工作流仍从 Secret 读取 | |
| `SSH_USER` | 已配置，必需 | SSH 账号；非敏感，当前工作流仍从 Secret 读取 | |
| `SSH_PRIVATE_KEY` | 已配置，必需 | 完整 PEM 私钥；更换服务器绑定密钥后同步更新 | |
| `SSH_KNOWN_HOSTS` | 已配置，必需 | 服务器主机公钥；可能因重装、换机器、换 IP / 端口而需更新，获取方式见第 4 节 | |
| `MONGO_PASSWORD` | 未配置，首次连接必需 | Mongo 密码；已有同一连接可复用服务器旧密码 | |
| `JWT_SECRET` | 未配置，可选 | JWT 签名密钥；默认由服务器生成 / 复用 | |
| `OSS_ACCESS_KEY_ID` | 未配置，上传功能需要 | 注入 `oss.access_key_id` | |
| `OSS_ACCESS_KEY_SECRET` | 未配置，上传功能需要 | 注入 `oss.access_key_secret` | |
| `SMS_ACCESS_KEY_ID` | 未配置，短信功能需要 | 注入 `auth.phone_code.access_key_id` | |
| `SMS_ACCESS_KEY_SECRET` | 未配置，短信功能需要 | 注入 `auth.phone_code.access_key_secret` | |
| `SMS_SECURITY_TOKEN` | 未配置，按凭证类型选填 | 注入 `auth.phone_code.security_token`；临时凭证才需要，默认留空 | |
| `DEEPSEEK_API_KEY` | 未配置，对应模型需要 | 注入 `deepseek.api_key` | |
| `DASHSCOPE_API_KEY` | 未配置，对应模型需要 | 注入 `dashscope.api_key` | |

**GitHub 页面：**进入 [Settings → Secrets and variables → Actions → Secrets](https://github.com/juemiaoluoji/picpac/settings/secrets/actions)。新增点 **New repository secret**；修改已有项点编辑按钮，填写新值后保存。PEM 须保留首尾标记及换行。

**命令行：**先 `gh auth login` 登录。以下命令会交互读取新值；同名 `set` 即更新，避免把真实密码放进命令参数和历史记录：

```powershell
gh secret set TCR_PASSWORD --repo juemiaoluoji/picpac
gh secret set MONGO_PASSWORD --repo juemiaoluoji/picpac
gh secret list --repo juemiaoluoji/picpac
```

从 PEM 文件上传，在 PowerShell 替换为自己的文件路径：

```powershell
Get-Content -Raw -LiteralPath 'C:\你的目录\部署密钥.pem' | gh secret set SSH_PRIVATE_KEY --repo juemiaoluoji/picpac
```

修改后按第 2 节重新部署。参见 [GitHub Secret 使用说明](https://docs.github.com/en/actions/how-tos/write-workflows/choose-what-workflows-do/use-secrets)与 [CLI 设置说明](https://cli.github.com/manual/gh_secret_set)。

## 8. Variables：说明、GitHub 页面与 CLI

Variable 用于地址、端口、服务名等普通配置，页面与 CLI 均可查看，不用于保存密码。

| 名称 | 状态 | 说明 | 值 |
| --- | --- | --- | --- |
| `TCR_IMAGE` | 已配置 | 镜像仓库地址，不带 tag | `ccr.ccs.tencentyun.com/picpac/server` |
| `K3S_CONFIG_PATH` | 已配置 | 服务器配置文件生成路径 | `/picpac/config/config.json` |
| `K3S_NODE_PORT` | 已配置 | 首次创建应用 Service 的 NodePort | `32090` |
| `SSH_PORT` | 未配置，使用默认值 | SSH 端口；更换后更新防火墙及主机公钥记录 | `22` |
| `TKE_PLATFORM` | 未配置，使用默认值 | 沿用原名称，实际是 k3s 节点镜像架构 | `linux/amd64` |
| `MONGO_SERVICE` | 未配置，已有配置可复用 | Mongo Service 完整 DNS，须查询真实名称与命名空间 | |
| `MONGO_PORT` | 未配置 | 先复用旧连接端口，无旧值时默认 `27017`；应填写实际 Service 内部端口 | |
| `MONGO_USERNAME` | 未配置，已有配置可复用 | Mongo 用户名；新连接必须填写 | |
| `MONGO_DATABASE` | 未配置，使用默认值 | 应用数据库 | `picpac` |
| `MONGO_AUTH_SOURCE` | 未配置 | 先保留同一连接的认证库，无旧值时默认 `admin`；不是必然等于应用数据库 | |
| `SMS_SIGN_NAME` | 未配置 | 短信签名，对应 `auth.phone_code.sign_name` | |
| `SMS_TEMPLATE_CODE` | 未配置 | 短信模板，对应 `auth.phone_code.template_code` | |
| `SMS_SCHEME_NAME` | 未配置，按短信产品填写 | 对应 `auth.phone_code.scheme_name` | |

**GitHub 页面：**进入 [Settings → Secrets and variables → Actions → Variables](https://github.com/juemiaoluoji/picpac/settings/variables/actions)。新增点 **New repository variable**；修改已有项点编辑按钮并保存。

**命令行：**同名 `set` 更新，`get` 查看单项，`list` 查看全部。Mongo 的值先按第 5 节查询，不能直接照抄示例。

```powershell
gh variable set TCR_IMAGE --repo juemiaoluoji/picpac --body 'ccr.ccs.tencentyun.com/picpac/server'
gh variable set K3S_NODE_PORT --repo juemiaoluoji/picpac --body '32090'
gh variable get TCR_IMAGE --repo juemiaoluoji/picpac
gh variable list --repo juemiaoluoji/picpac
```

工作流用 `${{ vars.名称 }}` 读取 Variable，用 `${{ secrets.名称 }}` 读取 Secret。两者同名也不会自动互相替代，迁移时需同步修改工作流；保存后重新部署生效。参见 [CLI Variable 设置说明](https://cli.github.com/manual/gh_variable_set)。
