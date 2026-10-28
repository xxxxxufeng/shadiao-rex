# shadiao-rex · 沙雕漫剧 rex 模块

在 [rex](https://github.com/InchStudio/ForwardWidgets)（原 ForwardWidget）里看红果的**沙雕画风**漫剧：热门、最新、分类、搜索、播放记录；看完的剧不再推荐，不含 AI 3D 剧。

由两部分组成：

- **hgsd**（`server/`）：自己部署的 Go 服务。用 [openhongguo](https://github.com/beilunyang/openhongguo) 以匿名设备调红果，聚合沙雕画风的漫剧、记录播放进度，并把红果 CENC 加密的 MP4 边拉边解密转给播放器（rex 无法自己解密）。
- **rex 模块**（`widgets/hongguo-shadiao.js`）：只调你的 hgsd。

> 仅供个人学习与自用。红果内容版权归原平台所有，请勿用于商业或公开分发。

## Docker 部署

镜像：`ghcr.io/xxxxxufeng/shadiao-rex:latest`，支持 `linux/amd64` 和 `linux/arm64`（群晖/威联通/Linux 服务器/Apple 芯片 Mac 均可），公开镜像，无需登录。

### 方式一：docker compose（推荐）

```sh
mkdir shadiao && cd shadiao
curl -O https://raw.githubusercontent.com/xxxxxufeng/shadiao-rex/main/docker-compose.yml
docker compose up -d
```

`docker-compose.yml` 内容如下，也可以手动创建：

```yaml
services:
  hgsd:
    image: ghcr.io/xxxxxufeng/shadiao-rex:latest
    container_name: hgsd
    restart: unless-stopped
    ports:
      - "8686:8686"
    volumes:
      - ./data:/data
```

### 方式二：docker run

```sh
docker run -d --name hgsd --restart unless-stopped \
  -p 8686:8686 -v $(pwd)/data:/data \
  ghcr.io/xxxxxufeng/shadiao-rex:latest
```

### 启动后

```sh
curl http://localhost:8686/healthz     # 返回 ok 即正常
cat data/config.json                   # 查看自动生成的访问口令 token
docker logs -f hgsd                    # 查看日志
```

### 获取访问口令（token）

首次启动时会自动生成一个 64 位的随机口令，保存在 `data/config.json` 的 `token` 字段。任选一种方式查看：

```sh
grep token data/config.json                 # 在 docker-compose.yml 所在目录执行
docker exec hgsd cat /data/config.json      # 不在该目录时，直接读容器里的
```

输出类似 `"token": "4f0c…e9a1"`，引号里那一串就是口令，填到 rex 模块设置的「访问口令」里。

想换口令：改 `data/config.json` 里的 `token`，然后 `docker compose restart`，rex 里同步改掉即可。

首次启动会在后台建「沙雕池」（搜索约 40 个关键词，**需要 5～10 分钟**），日志出现 `pool: refreshed, N shows` 之后，热门/最新/分类才有内容。

### 配置

首次启动在 `data/config.json` 生成配置，修改后执行 `docker compose restart`：

| 字段 | 默认 | 说明 |
|---|---|---|
| `token` | 随机生成 | 访问口令，rex 模块里填它。要换口令就改这里 |
| `listen` | `:8686` | 容器内监听地址，一般不用改；改宿主机端口请改 compose 的 `ports`，如 `"9000:8686"` |
| `publicBase` | 空 | 对外访问地址，如 `https://hg.example.com`。用来生成播放链接，**经反向代理访问时必须填** |
| `keywords` | 内置 40 个 | 沙雕池的搜索种子词 |
| `refreshHours` | `6` | 沙雕池刷新间隔（小时） |

### 数据目录

`data/` 下的文件，备份或迁移时整个目录拷走即可：

| 文件 | 内容 |
|---|---|
| `config.json` | 配置与口令 |
| `history.json` | 播放记录 |
| `pool.json` | 沙雕池缓存（删掉会在启动时重建） |
| `anonymous.json` / `meta.json` | 红果匿名设备档案（删掉会自动重新注册） |

容器启动时会自动把 `data/` 的属主改为容器内用户（uid 1000），再以该非 root 用户运行服务。

### 外网访问（HTTPS 反向代理）

口令会出现在播放链接里，外网使用请务必走 HTTPS。以 Caddy 为例：

```
hg.example.com {
    reverse_proxy 127.0.0.1:8686
}
```

然后在 `data/config.json` 里设置 `"publicBase": "https://hg.example.com"` 并重启容器。Nginx 等其他反代同理，注意不要缓存、不要限制响应大小（视频是流式返回的，支持 Range 拖动进度）。

只在家里局域网用的话不需要反代，服务地址直接填 `http://局域网IP:8686`。

### 升级

```sh
docker compose pull && docker compose up -d
```

`data/` 不受影响，口令和播放记录都会保留。

### 常见问题

- **rex 里列表是空的**：沙雕池还在建，看 `docker logs hgsd` 等出现 `pool: refreshed`。
- **rex 提示无法加载**：先在手机浏览器打开 `服务地址/healthz` 看是否返回 `ok`。打不开是网络问题；iOS 访问局域网 IP 需要在「设置 → 隐私与安全性 → 本地网络」里允许 rex。
- **点详情提示「请先在模块设置填写 server 和 token」**：先打开一次任意列表再点详情。
- **播放失败、日志里有 `anonymous response was empty`**：红果限制了匿名设备，服务会自动换新设备重试；偶发可忽略。
- **播放报 502 且持续出现**：可能红果改了接口，等镜像更新后 `docker compose pull` 升级。

## 在 rex 里使用

> 先按上文部署好 hgsd，并拿到「服务地址」和「访问口令」。

### 1. 模块下载地址

| 方式 | 地址 | 说明 |
|---|---|---|
| **订阅（推荐）** | `https://raw.githubusercontent.com/xxxxxufeng/shadiao-rex/main/shadiao-rex.fwd` | 模块索引，以后有新版本 rex 会提示更新 |
| 单个模块 | `https://raw.githubusercontent.com/xxxxxufeng/shadiao-rex/main/widgets/hongguo-shadiao.js` | 直接导入 JS 文件，升级时需要重新导入 |

### 2. 添加模块

打开 rex 的模块管理，选择添加订阅（或导入模块），粘贴上面的地址，在列表里安装「沙雕漫剧」。

### 3. 填写模块设置

在「沙雕漫剧」的模块设置里填三项：

| 设置项 | 填什么 | 例子 |
|---|---|---|
| 服务地址 | hgsd 的访问地址，带 `http://` 或 `https://` 和端口，**末尾不要加 `/`** | 局域网：`http://192.168.1.10:8686`<br>外网：`https://hg.example.com` |
| 访问口令 | `data/config.json` 里的 `token`，见[获取访问口令](#获取访问口令token) | `4f0c…e9a1`（64 位） |
| 清晰度 | 播放清晰度，默认 1080P | 流量紧张可选 720P / 480P |

填好后**先打开一次任意列表**（比如「沙雕热门」）。模块会在这时记住设置，之后点进详情页才能用。

### 4. 板块说明

| 板块 | 内容 |
|---|---|
| 沙雕热门 | 沙雕画风漫剧，按红果热度排序 |
| 沙雕最新 | 按上线时间排序 |
| 沙雕·搞笑 / 修仙玄幻 / 古代宫廷 / 都市日常 / 恐怖怪谈 / 系统升级 | 按题材分类；点板块右侧的 `>` 进入后，可切换「最热 / 最新」并翻页 |
| 播放记录 | 最近看过的剧，标题后面标着「看到第 N 集」「已看完」「有更新」 |

所有列表都已排除 AI 3D 剧，也不会出现你看完的剧。

### 5. 看剧

- 点开一部剧进入详情页：简介、标签、分集列表、相关推荐。
- 上次看到的那集会标成「▶ 第 N 集（上次看到）」，点它接着看。
- 点详情页里的标签（如「规则怪谈」），会列出带同一标签的其他沙雕剧。
- 支持拖动进度条。

### 6. 搜索

用 rex 自带的搜索，输入关键词后选择「沙雕漫剧」模块。只返回沙雕画风的剧，可以搜剧名，也可以搜题材（如「修仙」「规则」「大明」）。

### 7. 看完的规则

- **完结剧**：播放了最后一集，算「已看完」。
- **连载剧**：追到当前最新一集，算「追平」；之后更新了新集，播放记录里会显示「有更新」，这部剧也会重新出现在列表里。
- 看完和追平的剧不会再出现在热门、最新、分类、搜索和相关推荐里，在「播放记录」里仍然能找到。
- 播放记录保存在服务端的 `data/history.json`，换手机不会丢。

### 8. 更新模块

用订阅方式添加的，有新版本时在 rex 的模块管理里更新即可；单文件导入的，重新导入一次。服务端更新见上文「升级」。

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
