# 镜像构建与 TKE 人工发布

代码保留在自己的 GitHub 仓库；镜像发布到自己的腾讯云 **TCR 个人版私有仓库**。朋友的 GitHub 仓库不是镜像仓库，也无需将代码迁到腾讯云。

正式构建在 GitHub 的 `ubuntu-latest` runner 上完成，**你的电脑不需要 Docker**。构建阶段使用 `golang:1.25-bookworm`，运行阶段使用 `scratch`：静态 Go 程序不需要操作系统发行版或 shell，镜像只携带程序、分类配置和访问 HTTPS 所需的 CA 证书，并以非 root 用户运行。

## 当前默认行为

- `main` 分支提交、面向 `main` 的 PR，以及手动运行工作流，会执行测试、`go vet` 和 Linux 镜像构建。
- GitHub 变量和 Secrets 当前没有填入真实值。全部为空时只验证构建，不登录 TCR、不推送镜像，也不更新 TKE。
- 构建检查会验证镜像架构、非 root 用户、程序、分类配置和 CA 证书，并检查没有打包 `config.json` 或 Git 元数据。
- 验证模式的镜像只在 GitHub runner 的 Docker 中临时存在，任务结束后不保留。没有上传镜像 Artifact，也没有发布到 GHCR 或 Docker Hub。
- 发布后的版本是 `sha-<完整提交 SHA>`。每次发布产生独立版本，不覆盖 `latest`。
- 默认架构是 `linux/amd64`。ARM64 节点需要设置 `TKE_PLATFORM=linux/arm64`，或在手动运行时选择该架构。当前为单架构镜像，不适用于混合架构调度。

## 首次需要人工配置

1. 在自己的 GitHub 仓库启用 Actions，并把这些部署文件提交到该仓库。若为 GitHub fork，Actions 可能需要先手动启用。工作流合并到默认分支后，Actions 页面可使用 **Container image → Run workflow**。手动验证时保持 `publish=false`。
2. 在腾讯云 TCR 控制台选择个人版，初始化登录密码，创建命名空间及 `picpac` **私有**仓库。记下控制台提供的 Registry 域名及登录用户名。
3. 在自己的 GitHub 仓库 **Settings → Secrets and variables → Actions** 配置以下内容。现在可以全部不创建；以后需要发布时一次补齐，不要填假密码。

| 类型 | 名称 | 内容 |
| --- | --- | --- |
| Variable | `TCR_IMAGE` | 完整仓库路径，不带协议或标签，例如 `ccr.ccs.tencentyun.com/你的命名空间/picpac`；以控制台域名为准 |
| Secret | `TCR_USERNAME` | TCR 控制台提供的登录用户名 |
| Secret | `TCR_PASSWORD` | TCR 个人版初始化的登录密码 |
| Variable，可选 | `TKE_PLATFORM` | `linux/amd64` 或 `linux/arm64`，默认前者 |

4. 三项发布配置补齐后，提交到 `main` 会自动构建、验证并推送。也可以手动运行，勾选 `publish=true`。只填部分配置，或要求发布但未配齐，工作流会明确失败，不会把未发布的构建显示为发布成功。
5. 在 TKE 的应用命名空间创建镜像拉取凭证，配置相同的 Registry、用户名和密码，并让 `picpac` 工作负载引用该凭证。
6. 使用当前的 `config.json` 创建或更新 TKE 应用配置 Secret，把键 `config.json` 以只读文件挂载到 `/app/config/config.json`。镜像已设置 `PICPAC_CONFIG` 指向该文件，`category.json` 已随程序放在 `/app/category.json`。配置文件需要对容器用户 `10001` 可读（Secret 文件权限可使用 `0444`）。这里的运行配置 Secret 与 GitHub 的镜像仓库 Secrets 是不同用途；GitHub 凭证留空不影响本地构建。
7. 首次创建或更新 TKE Deployment 时设置镜像、配置挂载、匹配实际 `server.port` 的容器及 Service 端口、资源和探针。当前 `config.json` 使用 **9090**，Dockerfile 同样声明 `EXPOSE 9090`；该声明不会覆盖应用配置。可使用 TCP 启动检查和 `/api/v1/categories` 就绪检查；不应将依赖 MongoDB 的查询用于存活检查。

保留现有应用配置内容和配置方式，运行时通过挂载使用它。不要把本地 `config.json` 复制到镜像；`.dockerignore` 也会将它排除在构建上下文之外。当前本地配置中的 MongoDB URI 使用环回地址，**不能直接用于 TKE Pod**。在 TKE 配置文件中将连接地址改为你已准备好的 MongoDB 副本集地址，并确保副本集公布的成员地址也可达；这里保留本地配置，不修改或重新部署 MongoDB。

按项目提交规则，提交到 Git 的配置凭证必须使用 `偷apikey的替我挡灾` 占位；MongoDB URI 内的占位凭证使用 URL 编码。仓库内的占位配置不能直接用于生产，TKE 挂载的配置使用真实运行值，本地现有配置不加入这次提交。

## 每次更新需要人工做什么

1. 提交或合并代码到自己仓库的 `main`。
2. 等待 **Container image** 工作流成功，在结果摘要中复制已发布的完整镜像地址。只有显示 **Published to TCR** 才代表推送成功。
3. 在 TKE **工作负载 → picpac → 更新** 中换成该镜像版本。保留挂载、Service 和数据库设置，等待 Pod 就绪并调用接口确认。

也可在已有集群连接环境中执行（假设命名空间、Deployment 和容器均叫 `picpac`）：

```bash
kubectl -n picpac set image deployment/picpac \
  picpac=ccr.ccs.tencentyun.com/你的命名空间/picpac:sha-完整提交SHA
kubectl -n picpac rollout status deployment/picpac --timeout=180s
```

回退时在控制台改回上次成功镜像，或执行 `kubectl -n picpac rollout undo deployment/picpac` 后再次检查发布状态。应用版本回退不恢复数据库或配置内容。

## 本地构建及运行

本节仅供想在本地调试的开发者使用，不是 GitHub 镜像流程的前置步骤。选择本地调试时需要已安装并运行 Docker。以下为 Linux/macOS shell 示例，在仓库根目录执行：

```bash
docker buildx build --platform linux/amd64 --load -t picpac:local .
docker run --rm -p 9090:9090 \
  --mount "type=bind,source=$(pwd)/config.json,target=/app/config/config.json,readonly" \
  picpac:local
```

Windows PowerShell 运行示例：

```powershell
docker buildx build --platform linux/amd64 --load -t picpac:local .
$configPath = (Resolve-Path -LiteralPath config.json).Path
docker run --rm -p 9090:9090 --mount "type=bind,source=$configPath,target=/app/config/config.json,readonly" picpac:local
```

若以后修改配置端口，按实际端口调整映射。容器中的 `127.0.0.1` 指向容器自己；本地 MongoDB 位于 Docker Desktop 宿主机时，可在一份本地运行配置中使用 `host.docker.internal`，同时确保副本集公布的成员地址可达。不要仅为构建验证修改现有数据库配置。

## 免费使用范围

TCR 个人版是限额免费服务；GitHub Free 私有仓库的标准 runner 有每月免费额度，公开仓库的标准 runner 免费。工作流关闭 Go 缓存、Docker build-record Artifact 上传，不保留大型镜像产物。设置 GitHub Actions 付费预算为 0，并启用预算耗尽停止；额度耗尽时等待重置或本地构建。

镜像流程不新建 CLB、节点、数据库或付费日志资源。TKE 现有运行资源、业务流量、OSS、短信和模型接口的费用不属于免费镜像工具链。

官方参考：[TCR 个人版快速入门](https://cloud.tencent.com/document/product/1141/63910)、[TCR 计费](https://cloud.tencent.com/document/product/1141/40540)、[GitHub Actions 计费](https://docs.github.com/en/billing/concepts/product-billing/github-actions)、[GitHub 预算设置](https://docs.github.com/en/billing/how-tos/set-up-budgets)。
