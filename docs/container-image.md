# GitHub 镜像构建与 k3s 自动部署

代码保留在自己的 GitHub 仓库，镜像发布到 TCR 个人版私有仓库，应用运行在已有 k3s 上。正式构建由 GitHub 托管 runner 完成，电脑和服务器无需安装自托管 runner。

构建镜像为 `golang:1.25-bookworm`，运行镜像为 `scratch`。运行镜像包含静态程序、`category.json` 和 CA 证书，以用户 10001 运行。`config.json` 不进入构建上下文或镜像。

## 发布行为

- `main` 提交自动执行测试、go vet、镜像构建和内容检查。
- 配齐 TCR 配置后推送镜像，标签为提交 hash 的前 7 位，例如 `ccr.ccs.tencentyun.com/picpac/server:f6b2854`。
- 推送成功后调用 `Deploy to k3s` 工作流，使用 SSH 登录服务器，通过 `sudo -n -i` 执行部署。
- `picpac` 命名空间没有 `picpac` Deployment 时创建；存在时更新名为 `picpac` 的容器镜像和配置挂载，保留现有副本数、资源设置、其他容器和 Service。
- 没有 Service 时创建 NodePort Service：默认外部端口 32090，目标端口读取运行配置，当前为 9090。
- 每次部署从仓库读取 `config.json`，叠加 `PICPAC_CONFIG_OVERRIDES` 的运行凭证，自动写入服务器指定路径，再同步到 `picpac-config` Secret。TCR 拉取凭证同步到 `tcr-pull` Secret；配置内容变化时触发 Pod 更新。
- 等待 Deployment 就绪，失败时工作流报错，不自动删除工作负载、不自动回滚数据库。
- PR 只做构建和部署脚本检查，不能推镜像或部署服务器。
- 全部 TCR 配置为空时仅验证构建。手动运行可保持 `publish=false`；需要只推镜像时设置 `publish=true`、`deploy=false`。

## GitHub 配置

配置位置：自己的仓库 **Settings → Secrets and variables → Actions**。

| 类型 | 名称 | 内容 |
| --- | --- | --- |
| Variable | `TCR_IMAGE` | 完整仓库路径，不带协议和标签，当前为 `ccr.ccs.tencentyun.com/picpac/server` |
| Secret | `TCR_USERNAME` | TCR 登录指引中的账号 ID |
| Secret | `TCR_PASSWORD` | TCR 个人版访问密码 |
| Secret，可选 | `PICPAC_CONFIG_OVERRIDES` | 未配置时按空覆盖对象处理；后续可填写运行配置覆写 JSON，不将真实值提交到代码仓库 |
| Secret | `SSH_HOST` | k3s 服务器公网地址 |
| Secret | `SSH_USER` | 已绑定密钥的账号，例如 ubuntu；必须可免交互提权 |
| Secret | `SSH_PRIVATE_KEY` | PEM 私钥完整内容，保留首尾标记和换行 |
| Secret | `SSH_KNOWN_HOSTS` | 经核验的服务器 SSH 主机公钥记录；不能通过关闭 StrictHostKeyChecking 代替 |
| Variable | `K3S_CONFIG_PATH` | 服务器上的生产配置文件，当前为 `/picpac/config/config.json`；默认 `/etc/picpac/config.json` |
| Variable，可选 | `K3S_NODE_PORT` | 首次创建 Service 的 NodePort，默认 `32090`，范围 30000–32767 |
| Variable，可选 | `MONGO_NODE_PORT` | 当仓库 Mongo URI 使用环回地址时，用该 NodePort 查找实际 Mongo Service；默认 `32017` |
| Variable，可选 | `SSH_PORT` | SSH 监听端口，默认 `22` |
| Variable，可选 | `TKE_PLATFORM` | 保留原构建变量名，默认 `linux/amd64`；ARM 节点使用 `linux/arm64` |

这些配置不会写进代码。Secrets 不填假密码；TCR 发布配置必须完整，SSH 部署配置也必须完整。

## 首次服务器准备

1. 保证 GitHub runner 能连接 SSH 端口。GitHub 托管 runner 出口 IP 不固定，仅允许个人电脑来源 IP 时，它无法直接 SSH 部署。
2. 在目标登录账号下验证 `sudo -n -i /usr/local/bin/k3s kubectl get nodes`。该命令必须无需密码、无需交互；流水线不修改 sudo 权限。
3. 服务器无需提前创建 JSON；每次部署自动创建或覆盖 `K3S_CONFIG_PATH`，权限为 600。未设置额外配置 Secret 时，JWT 在服务器本地生成并复用；Mongo 凭证在服务器上从现有 Mongo Pod 环境或其引用的 Secret 中读取，不传回 GitHub、不修改 Mongo。若现有 Mongo 凭证无法发现，部署明确失败。
4. Mongo URI 应使用 Service 地址或可达内网地址。若输入仍是 localhost/127.0.0.1，脚本会查找唯一一个 NodePort 32017 对应的 Mongo Service，并转换成集群 DNS 地址，保留账号、数据库及副本集参数；找不到时明确失败。不会修改已有 Mongo Service 或数据。
5. 为外部访问在**实例防火墙**放行 TCP 32090。修改模板不会自动修改既有实例规则。SSH 的 22 与 Mongo 的 32017 各有独立用途。

配置流向为：

```text
仓库 config.json + GitHub Secret PICPAC_CONFIG_OVERRIDES
  → 服务器 /picpac/config/config.json（自动写入）
  → k3s Secret picpac-config
  → 容器 /app/config/config.json
```

运行配置会转换为无 BOM 的 UTF-8 JSON。JWT 本地生成并保存在配置目录的 `.picpac-jwt-secret`，不会每次重建；已经有效的服务器 JWT 和 Mongo URI 优先复用。OSS、短信和 AI 的占位 Key 可在后续通过配置覆写补齐；未配置真实 Key 时，这些外部服务功能不可用。镜像已设置 `PICPAC_CONFIG` 指向挂载文件，`category.json` 位于 `/app/category.json`。Secret 文件权限为 0444，容器用户 10001 可读。旧 Pod 的 subPath 配置不会直接热更新；部署使用配置校验值触发新 Pod。修改已有应用端口时，要同步调整已有 Deployment 和 Service。

## 手动重试部署，不重新构建镜像

Actions → **Deploy to k3s → Run workflow**，填写已经发布的完整镜像地址及架构。该工作流也接受原来的 `sha-完整hash` 版本，方便使用已上传镜像验证首次部署或回退。

命令行示例：

```bash
gh workflow run deploy-k3s.yml --repo juemiaoluoji/picpac \
  --ref main -f image=ccr.ccs.tencentyun.com/picpac/server:七位hash \
  -f platform=linux/amd64
```

工作流尚未合并 main 时，选择或指定 `codex/container-image` 分支；部署文件在 PR 检查中注册后，可通过 CLI 手动触发该分支。PR 仍保持草稿时不会因为 PR 检查部署服务器。

## 检查运行结果

```bash
sudo -i
k3s kubectl -n picpac get deployment,pods,svc
k3s kubectl -n picpac logs deployment/picpac --tail=50
curl http://127.0.0.1:32090/api/v1/categories
```

外部访问当前为 `http://服务器公网IP:32090/api/v1/categories`。已有 Service 会保留，以 `get svc` 输出的实际端口为准。首次部署有 TCP 启动检查、TCP 存活检查和分类查询就绪检查；只有 Pod 就绪后才显示部署成功。

## 免费使用及验证

TCR 个人版限额免费，GitHub 标准 runner 使用免费规则或配额。工作流关闭大型 Artifact 和 Go 缓存，不新增云服务器、CLB、数据库或付费日志服务。现有服务器、业务流量、OSS、短信和模型接口的费用不属于镜像工具链。

部署测试：`python3 -m unittest discover -s deploy/tests -v`。脚本测试使用模拟 kubectl，覆盖首次创建、已有 Deployment 更新、缺失配置、Mongo 环回地址、非法镜像和端口、启动失败及旧标签兼容，不连接真实集群。

参考：[GitHub Secrets](https://docs.github.com/en/actions/how-tos/write-workflows/choose-what-workflows-do/use-secrets)、[k3s 集群访问](https://docs.k3s.io/cluster-access)、[私有镜像拉取](https://kubernetes.io/docs/tasks/configure-pod-container/pull-image-private-registry/)。
