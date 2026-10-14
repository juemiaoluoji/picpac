# 阿里云号码认证原子能力

`config.AliyunPNVSConfig` 和 `service.PhoneVerificationService` 的生产实现为
`service.AliyunPhoneVerificationService`，已装配到发送验证码、手机号登录和已登录用户重置密码接口。
重置密码原样复用发送能力及同一方案，验证码不区分登录与重置用途。

## 调用方式

由调用方构造配置并传给 `service.NewAliyunPhoneVerificationService`，构造失败返回错误。
`AccessKeyID`、`AccessKeySecret`、`SignName`、`TemplateCode` 必填；`SecurityToken` 用于
STS 临时凭证，`SchemeName` 可选且最长 20 个字符。应用从 `config.json` 的
`auth.phone_code` 读取 `access_key_id`、`access_key_secret`、`security_token`、
`scheme_name`、`sign_name`、`template_code`、`request_timeout_seconds`、
`valid_time_seconds`、`interval_seconds` 和 `duplicate_policy`，生产配置缺失时启动失败。
运行参数默认分别为请求超时 10 秒、验证码有效期 300 秒、发送间隔 60 秒和覆盖旧验证码；
`duplicate_policy` 只允许 `1`（覆盖）或 `2`（保留）。

顶层 `env` 必须为 `dev` 或 `prod`，修改后重启应用生效：

- `dev`：不创建 PNVS 服务，不要求 PNVS 凭证。发送接口直接返回成功；登录校验
  `auth.phone_code.dev_fixed_code`，默认值为 `123456`，无需先发送。用户创建和 token 签发保持原流程。
- `prod`：发送和核验均直接调用 PNVS，不在 MongoDB 保存验证码或验证码状态。

运行模式仅由 `env` 决定。验证码有效期、发送间隔、重复发送覆盖和核验均由 PNVS 管理。

`SchemeName` 留空时，请求会省略该参数以使用默认方案；显式传空字符串可能触发
`lc.ASSERT_ERROR`（`header.schemeName` 非空断言失败）。

- `SendCode(ctx, phone) error`：调用 `SendSmsVerifyCode`，由阿里云生成 6 位数字验证码；
  有效期、发送间隔和重复发送策略使用配置值，短信模板中的分钟数根据有效期自动换算，
  不返回验证码明文。
- `VerifyCode(ctx, phone, code) (bool, error)`：调用 `CheckSmsVerifyCode`；仅 `PASS`
  返回 `true, nil`，`UNKNOWN` 返回 `false, nil`；调用失败或响应异常返回错误。

手机号支持国内 11 位号码和 `+86` 前缀；验证码为 6 位 ASCII 数字。
模板需使用 `code`、`min` 变量，发送参数为 `{"code":"##code##","min":"5"}`。
签名和模板需在号码认证控制台开通并匹配。配置按服务实例保存，发送与核验使用相同方案。

使用官方 Go SDK `github.com/alibabacloud-go/dypnsapi-20170525/v3 v3.0.0`，
通过 HTTPS POST 调用 `dypnsapi.aliyuncs.com`，API 版本为 `2017-05-25`。
请求序列化、ACS3-HMAC-SHA256 签名和响应解析由 SDK 处理。
通过 SDK 的自定义 HTTP 客户端传递 context，客户端超时由配置控制且默认 10 秒，
不自动重试 HTTP 请求。
每次调用创建独立的 SDK 客户端以隔离 context，底层 HTTP 连接池共享。
错误向上传递，不在原子能力内记录日志。SDK 错误的文本可能包含完整请求 URL，
对外错误字符串不包含该文本，原始错误通过 `Unwrap` 保留，调用方不要直接记录原始错误。

认证流程使用 `PhoneVerificationService` 发送及核验，不再装配 `FakeSMSService`。
账号处理和 token 签发仍由业务层负责。

## 官方依据

- [发送验证码](https://help.aliyun.com/zh/pnvs/developer-reference/api-dypnsapi-2017-05-25-sendsmsverifycode)
- [核验验证码](https://help.aliyun.com/zh/pnvs/developer-reference/api-dypnsapi-2017-05-25-checksmsverifycode)
- [官方 SDK 源码](https://github.com/alibabacloud-go/dypnsapi-20170525/tree/v3.0.0)

测试使用本地模拟 HTTP Transport，不发送真实短信。
