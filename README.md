# shadiao-rex · 沙雕漫剧 rex 模块

在 [rex](https://github.com/InchStudio/ForwardWidgets)（原 ForwardWidget）里看红果的**沙雕画风**漫剧：热门、最新、分类、搜索、播放记录；看完的剧不再推荐，不含 AI 3D 剧。

由两部分组成：

- **hgsd**（`server/`）：自己部署的 Go 服务。用 [openhongguo](https://github.com/beilunyang/openhongguo) 以匿名设备调红果，聚合沙雕画风的漫剧、记录播放进度，并把红果 CENC 加密的 MP4 边拉边解密转给播放器（rex 无法自己解密）。
- **rex 模块**（`widgets/hongguo-shadiao.js`）：只调你的 hgsd。

> 仅供个人学习与自用。红果内容版权归原平台所有，请勿用于商业或公开分发。

## 部署 hgsd

```sh
mkdir shadiao && cd shadiao
curl -O https://raw.githubusercontent.com/xxxxxufeng/shadiao-rex/main/docker-compose.yml
docker compose up -d
cat data/config.json   # 首次启动自动生成访问口令 token
```

镜像：`ghcr.io/xxxxxufeng/shadiao-rex:latest`（amd64 / arm64）。首次启动后台会花几分钟建沙雕池。

`data/config.json`（改完 `docker compose restart`）：

| 字段 | 说明 |
|---|---|
| `token` | 访问口令，rex 模块里填它 |
| `publicBase` | 对外地址，例如 `https://hg.example.com`。生成播放链接用；留空则用请求的 Host |
| `keywords` | 沙雕池的搜索种子词，留空用内置的 40 个 |
| `refreshHours` | 沙雕池刷新间隔（小时），默认 6 |

要在外网用，请在前面加一层 HTTPS 反向代理（Caddy / Nginx 等）——口令会出现在播放链接里。

## 在 rex 里使用

1. 导入 `widgets/hongguo-shadiao.js`（可用 raw 链接）。
2. 模块设置里填「服务地址」（如 `http://192.168.1.10:8686` 或你的 `publicBase`）和「访问口令」。
3. 先打开一次任意列表（详情页靠它记住设置）。
4. 搜索用 rex 自带的搜索页，选本模块。

## 说明

- 红果匿名设备用一阵会被限制，hgsd 会自动换新设备。
- "沙雕画风"靠红果的画风标签（`沙雕` / `表情包`）判断，AI 3D（`3D` / `3D漫`）全部排除。
- 播放记录存在 `data/history.json`：完结剧播过最后一集算看完，连载剧追到最新集算追平，二者都不再推荐。

## 开发

```sh
cd server && go test ./...               # 解密相关测试需要样本，见 server/testdata/README.md
go test -tags live ./internal/upstream/  # 走真实红果接口
node test-hongguo-shadiao.js             # rex 模块离线测试
docker build -t shadiao-rex .
```
