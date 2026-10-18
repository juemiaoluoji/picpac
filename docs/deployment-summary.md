# Picpac 部署手册

仓库：`juemiaoluoji/picpac`；环境：腾讯云 Ubuntu + k3s；[PR #1](https://github.com/juemiaoluoji/picpac/pull/1) 合并后启用 main 自动流程，合并前手动选择 `codex/container-image` 分支。

## 部署流程

| 步骤 | 说明 |
| --- | --- |
| 1. 自动构建 | 代码合入 `main` 后自动触发 `.github/workflows/container-image.yml`，使用根目录 `Dockerfile` 构建镜像（构建基础镜像 `golang:1.25-bookworm`，运行基础镜像 `scratch`）。 |
| 2. 手动执行 | 在 GitHub **Actions → Container image → Run workflow** 开启 `publish` 和 `deploy`，即可手动构建、推送并部署，也可通过 **Deploy to k3s** 直接部署已有镜像。 |
| 3. 推送镜像 | 使用 Secrets 中的 `TCR_USERNAME`、`TCR_PASSWORD` 推送到限额免费的腾讯云个人版仓库 `ccr.ccs.tencentyun.com/picpac/server`，tag 为提交 SHA 前 7 位。 |
| 4. 部署服务 | Actions 使用 `SSH_HOST`、`SSH_USER`、`SSH_PRIVATE_KEY` 登录服务器并通过 `SSH_KNOWN_HOSTS` 校验身份，以免密码 sudo 操作 k3s 拉取镜像，无服务时创建、有服务时更新镜像和配置。 |
| 5. 生成配置 | 每次部署从 `config.prod.json` 注入各项 Secret、Variable，生成服务器 `/picpac/config/config.json` 并挂载给容器，配置不打进镜像。 |
| 6. 访问接口 | 容器监听 `9090`，实例防火墙放行 TCP `32090` 后，通过 `http://43.142.86.222:32090/api/v1/categories` 访问。 |

当前公开仓库使用标准 GitHub Runner 免费，GitHub Free 私有仓库额度为每月 **2,000 分钟，不是小时**；TCR 个人版默认每仓库最多 100 个版本，服务器及业务费用另算（[GitHub](https://docs.github.com/en/billing/concepts/product-billing/github-actions)、[腾讯云免费说明](https://cloud.tencent.com/document/product/1141/39278)、[版本配额](https://cloud.tencent.com/document/faq/1141/57780)）。

## Secrets

保存后加密存储到 GitHub，无法查看原值，只能更新；表内不记录真实凭证。

| 名称 | 状态 | 说明 | 值 |
| --- | --- | --- | --- |
| `TCR_USERNAME` | 已配置 | 腾讯云镜像仓库登录用户名 / 账号 ID | |
| `TCR_PASSWORD` | 已配置 | 镜像仓库访问密码，推送和 k3s 拉取共用 | |
| `SSH_HOST` | 已配置 | 服务器地址，当前工作流从 Secret 读取 | |
| `SSH_USER` | 已配置 | SSH 登录账号，当前为 ubuntu，需免密码 sudo | |
| `SSH_PRIVATE_KEY` | 已配置 | 已绑定服务器公钥对应的完整 PEM 私钥，不是文件路径；更换绑定密钥后更新 | |
| `SSH_KNOWN_HOSTS` | 已配置 | 服务器主机公钥记录；重装、换机器、换 IP / SSH 端口时需核对更新，普通重启或部署不变 | |
| `MONGO_PASSWORD` | 未配置 | Mongo 密码，首次连接必填；已有同一连接可复用服务器旧密码 | |
| `JWT_SECRET` | 未配置，可选 | JWT 签名密钥，默认由服务器生成并复用 | |
| `OSS_ACCESS_KEY_ID` | 未配置 | OSS 上传访问密钥 ID | |
| `OSS_ACCESS_KEY_SECRET` | 未配置 | OSS 上传访问密钥 Secret | |
| `SMS_ACCESS_KEY_ID` | 未配置 | 短信访问密钥 ID | |
| `SMS_ACCESS_KEY_SECRET` | 未配置 | 短信访问密钥 Secret | |
| `SMS_SECURITY_TOKEN` | 未配置，可选 | 使用临时凭证时填写，否则留空 | |
| `DEEPSEEK_API_KEY` | 未配置 | DeepSeek 调用密钥 | |
| `DASHSCOPE_API_KEY` | 未配置 | DashScope 调用密钥 | |

**页面修改：**[Settings → Secrets and variables → Actions → Secrets](https://github.com/juemiaoluoji/picpac/settings/secrets/actions)，新增点 **New repository secret**，修改点对应项的编辑按钮。

**命令行修改：**先 `gh auth login`，`set` 同名即更新并交互输入新值；PEM 可从文件上传（[CLI 说明](https://cli.github.com/manual/gh_secret_set)）。

```powershell
gh secret set MONGO_PASSWORD --repo juemiaoluoji/picpac
gh secret list --repo juemiaoluoji/picpac
Get-Content -Raw -LiteralPath 'C:\你的目录\部署密钥.pem' | gh secret set SSH_PRIVATE_KEY --repo juemiaoluoji/picpac
```

`SSH_KNOWN_HOSTS` 的本机已信任记录可用 `ssh-keygen -F 43.142.86.222` 查看；未配置真实 OSS、短信或 AI 密钥时，对应功能不可用。

## Variables

用于非敏感配置，可直接查看值；Mongo 使用集群内 Service DNS 和端口，只有密码需要 Secret。

| 名称 | 状态 | 说明 | 值 |
| --- | --- | --- | --- |
| `TCR_IMAGE` | 已配置 | 镜像仓库地址，不含 tag | `ccr.ccs.tencentyun.com/picpac/server` |
| `K3S_CONFIG_PATH` | 已配置 | 服务器配置文件路径 | `/picpac/config/config.json` |
| `K3S_NODE_PORT` | 已配置 | 首次创建应用 Service 的对外端口，已有 Service 保持原值 | `32090` |
| `SSH_PORT` | 未配置，默认生效 | SSH 端口，变更后同步修改防火墙和主机公钥记录 | `22` |
| `TKE_PLATFORM` | 未配置，默认生效 | 沿用原名称，实际为 k3s 镜像架构 | `linux/amd64` |
| `MONGO_SERVICE` | 未配置 | Mongo Service 完整 DNS：`服务名.命名空间.svc.cluster.local`；已有连接可复用旧值 | |
| `MONGO_PORT` | 未配置 | Service 内部端口，复用旧值或默认 `27017`，不是外部 `32017` | |
| `MONGO_USERNAME` | 未配置 | Mongo 用户名，首次连接必填，已有同一连接可复用 | |
| `MONGO_DATABASE` | 未配置，默认生效 | 应用数据库 | `picpac` |
| `MONGO_AUTH_SOURCE` | 未配置 | 复用原认证库，无旧值时默认 `admin` | |
| `SMS_SIGN_NAME` | 未配置 | 短信签名 | |
| `SMS_TEMPLATE_CODE` | 未配置 | 短信模板编号 | |
| `SMS_SCHEME_NAME` | 未配置，按需填写 | 短信认证方案名称 | |

**页面修改：**[Settings → Secrets and variables → Actions → Variables](https://github.com/juemiaoluoji/picpac/settings/variables/actions)，新增点 **New repository variable**，修改点对应项的编辑按钮。

**命令行修改：**`set` 同名即更新，`get` 查看单项，`list` 查看全部（[CLI 说明](https://cli.github.com/manual/gh_variable_set)）。

```powershell
gh variable set K3S_NODE_PORT --repo juemiaoluoji/picpac --body '32090'
gh variable get K3S_NODE_PORT --repo juemiaoluoji/picpac
gh variable list --repo juemiaoluoji/picpac
```

在服务器用 `sudo -i` 后执行 `k3s kubectl get svc -A` 查询 Mongo 名称与端口，旧 `MONGO_NODE_PORT` 已停用可删除；修改 Secret / Variable 后需重新部署生效。
