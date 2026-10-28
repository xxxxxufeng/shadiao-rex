WidgetMetadata = {
  id: "xufeng.hongguo.shadiao",
  title: "沙雕漫剧",
  version: "1.2.7",
  requiredVersion: "0.0.1",
  description: "热门、最新、分类、搜索、播放记录，看完不再推荐，不含 AI 剧",
  author: "xufeng",
  site: "https://github.com/xxxxxufeng/shadiao-rex",
  icon: "https://raw.githubusercontent.com/xxxxxufeng/shadiao-rex/main/icon.png",
  iconurl: "https://raw.githubusercontent.com/xxxxxufeng/shadiao-rex/main/icon.png", // rex reads this key
  detailCacheDuration: 60,
  globalParams: [
    {
      name: "server",
      title: "服务地址",
      type: "input",
      description: "hgsd 地址，例如 http://1.2.3.4:8686",
      placeholders: [{ title: "示例", value: "http://1.2.3.4:8686" }],
    },
    { name: "token", title: "访问口令", type: "input", description: "hgsd 的 config.json 里的 token" },
    {
      name: "quality",
      title: "清晰度",
      type: "enumeration",
      value: "1080p",
      enumOptions: [
        { title: "1080P", value: "1080p" },
        { title: "720P", value: "720p" },
        { title: "480P", value: "480p" },
      ],
    },
  ],
  modules: [
    {
      id: "hot",
      title: "沙雕热门",
      functionName: "loadHot",
      cacheDuration: 1800,
      params: [{ name: "page", title: "页码", type: "page" }],
    },
    {
      id: "new",
      title: "沙雕最新",
      functionName: "loadNew",
      cacheDuration: 1800,
      params: [{ name: "page", title: "页码", type: "page" }],
    },
    {
      id: "cat_funny",
      title: "沙雕·搞笑",
      functionName: "loadCategory",
      cacheDuration: 1800,
      params: [
        { name: "cat", title: "分类", type: "constant", value: "funny" },
        {
          name: "sort",
          title: "排序",
          type: "enumeration",
          value: "hot",
          enumOptions: [
            { title: "最热", value: "hot" },
            { title: "最新", value: "new" },
          ],
        },
        { name: "page", title: "页码", type: "page" },
      ],
    },
    {
      id: "cat_xianxia",
      title: "沙雕·修仙玄幻",
      functionName: "loadCategory",
      cacheDuration: 1800,
      params: [
        { name: "cat", title: "分类", type: "constant", value: "xianxia" },
        {
          name: "sort",
          title: "排序",
          type: "enumeration",
          value: "hot",
          enumOptions: [
            { title: "最热", value: "hot" },
            { title: "最新", value: "new" },
          ],
        },
        { name: "page", title: "页码", type: "page" },
      ],
    },
    {
      id: "cat_ancient",
      title: "沙雕·古代宫廷",
      functionName: "loadCategory",
      cacheDuration: 1800,
      params: [
        { name: "cat", title: "分类", type: "constant", value: "ancient" },
        {
          name: "sort",
          title: "排序",
          type: "enumeration",
          value: "hot",
          enumOptions: [
            { title: "最热", value: "hot" },
            { title: "最新", value: "new" },
          ],
        },
        { name: "page", title: "页码", type: "page" },
      ],
    },
    {
      id: "cat_urban",
      title: "沙雕·都市日常",
      functionName: "loadCategory",
      cacheDuration: 1800,
      params: [
        { name: "cat", title: "分类", type: "constant", value: "urban" },
        {
          name: "sort",
          title: "排序",
          type: "enumeration",
          value: "hot",
          enumOptions: [
            { title: "最热", value: "hot" },
            { title: "最新", value: "new" },
          ],
        },
        { name: "page", title: "页码", type: "page" },
      ],
    },
    {
      id: "cat_horror",
      title: "沙雕·恐怖怪谈",
      functionName: "loadCategory",
      cacheDuration: 1800,
      params: [
        { name: "cat", title: "分类", type: "constant", value: "horror" },
        {
          name: "sort",
          title: "排序",
          type: "enumeration",
          value: "hot",
          enumOptions: [
            { title: "最热", value: "hot" },
            { title: "最新", value: "new" },
          ],
        },
        { name: "page", title: "页码", type: "page" },
      ],
    },
    {
      id: "cat_system",
      title: "沙雕·系统升级",
      functionName: "loadCategory",
      cacheDuration: 1800,
      params: [
        { name: "cat", title: "分类", type: "constant", value: "system" },
        {
          name: "sort",
          title: "排序",
          type: "enumeration",
          value: "hot",
          enumOptions: [
            { title: "最热", value: "hot" },
            { title: "最新", value: "new" },
          ],
        },
        { name: "page", title: "页码", type: "page" },
      ],
    },
    {
      id: "history",
      title: "播放记录",
      functionName: "loadHistory",
      cacheDuration: 0,
      params: [],
    },
    {
      // rex resumes "continue watching" by asking this module for streams
      id: "loadResource",
      title: "播放",
      functionName: "loadResource",
      type: "stream",
      cacheDuration: 0,
      params: [],
    },
  ],
  search: {
    title: "搜索",
    functionName: "search",
    params: [
      { name: "keyword", title: "关键词", type: "input" },
      { name: "page", title: "页码", type: "page" },
    ],
  },
};

const HG_TAG = "[沙雕漫剧]";
const HG_CONF_KEY = "hg.conf";
const HG_LINK_RE = /^hg:(\d{6,25})$/;

// Handlers that get params remember the config, because loadDetail(link) gets none.
function hgConfig(params) {
  let conf = null;
  if (params && params.server && params.token) {
    conf = {
      server: String(params.server).trim().replace(/\/+$/, ""),
      token: String(params.token).trim(),
      quality: params.quality || "1080p",
    };
    Widget.storage.set(HG_CONF_KEY, JSON.stringify(conf));
  } else {
    const raw = Widget.storage.get(HG_CONF_KEY);
    if (raw) conf = typeof raw === "string" ? JSON.parse(raw) : raw;
  }
  if (!conf || !conf.server || !conf.token) {
    throw new Error(HG_TAG + " 请先在模块设置填写 server 和 token");
  }
  return conf;
}

async function hgGet(conf, path, query) {
  const res = await Widget.http.get(conf.server + path, {
    // /api/search uses q for the keyword; elsewhere q is the quality
    params: { q: conf.quality, ...(query || {}), t: conf.token },
  });
  if (!res || res.data == null) throw new Error(HG_TAG + " 服务无响应: " + path);
  if (res.data.error) {
    const who = res.data.source === "upstream" ? "红果接口失败" : "服务出错";
    throw new Error(HG_TAG + " " + who + ": " + res.data.error);
  }
  return res.data;
}

function hgDate(unix) {
  if (!unix) return undefined;
  const d = new Date(unix * 1000 + 8 * 3600 * 1000); // Asia/Shanghai
  return d.toISOString().slice(0, 10);
}

function hgItem(s, title) {
  const link = "hg:" + s.id;
  const tags = s.tags || [];
  return {
    id: link,
    type: "url",
    link,
    mediaType: "tv",
    title: title || s.title,
    description: s.description,
    coverUrl: s.cover,
    posterPath: s.cover,
    releaseDate: hgDate(s.created),
    genreItems: tags.map((t) => ({ id: t, title: t })),
    genreTitle: tags.join(" / "),
  };
}

async function hgList(params, kind, cat) {
  try {
    const conf = hgConfig(params);
    const query = { kind, page: Number(params.page || 1) };
    if (cat) query.cat = cat;
    // tapping a genre on a detail page reopens the list with genreId = the tag
    if (params.genreId) query.tag = String(params.genreId);
    const data = await hgGet(conf, "/api/list", query);
    return data.map((s) => hgItem(s));
  } catch (error) {
    console.error(HG_TAG, "list", kind, error.message || error);
    throw error;
  }
}

async function loadHot(params = {}) {
  return hgList(params, "hot");
}

async function loadNew(params = {}) {
  return hgList(params, "new");
}

async function loadCategory(params = {}) {
  return hgList(params, params.sort === "new" ? "new" : "hot", params.cat || "funny");
}

async function loadHistory(params = {}) {
  try {
    const conf = hgConfig(params);
    const data = await hgGet(conf, "/api/history");
    return data.map((s) => {
      let suffix = " · 看到第" + s.lastEpisode + "集";
      if (s.completed) suffix = " · 已看完";
      else if (s.hasUpdate) suffix = " · 有更新";
      return hgItem(s, s.title + suffix);
    });
  } catch (error) {
    console.error(HG_TAG, "history", error.message || error);
    throw error;
  }
}

async function search(params = {}) {
  try {
    const conf = hgConfig(params);
    const keyword = String(params.keyword || "").trim();
    if (!keyword) return [];
    const data = await hgGet(conf, "/api/search", { q: keyword, page: Number(params.page || 1) });
    return data.map((s) => hgItem(s));
  } catch (error) {
    console.error(HG_TAG, "search", error.message || error);
    throw error;
  }
}

async function loadDetail(link, params) {
  const ln = typeof link === "string" ? link : "";
  const em = HG_EP_LINK_RE.exec(ln);
  if (em) {
    const conf = hgLinkConfig(em, params);
    return conf ? hgEpisodeDetail(conf, em[1], Number(em[2]), ln) : null;
  }
  const m = HG_LINK_RE.exec(ln);
  if (!m) return null;
  try {
    const conf = hgConfig(params && params.server ? params : undefined);
    const s = await hgGet(conf, "/api/series/" + m[1]);
    const item = hgItem(s);
    // rex starts playback at `episode`: resume where the user left off, else from 1
    item.episode = s.lastEpisode || 1;
    item.episodeItems = (s.episodeList || []).map((e) => ({
      id: item.id + ":" + e.no,
      link: hgEpisodeLink(m[1], e.no, conf),
      type: "url",
      mediaType: "tv",
      title: e.no === s.lastEpisode ? "▶ 第" + e.no + "集（上次看到）" : "第" + e.no + "集",
      // no `episode` field: rex points its play button at the highest-numbered
      // episode, which would start unwatched series at the latest episode
      episodeName: "第" + e.no + "集",
      duration: e.duration,
      coverUrl: s.cover,
      videoUrl: e.url,
      playerType: "app",
    }));
    item.relatedItems = (s.related || []).map((r) => hgItem(r));
    return item;
  } catch (error) {
    console.error(HG_TAG, "detail", link, error.message || error);
    throw error;
  }
}

// episode link: hg:<series>:<episode>[|<server>|<token>] (URI-encoded parts).
// rex forgets Widget.storage on relaunch but hands this link back when it
// resumes continue-watching, so the link carries the server it came from.
const HG_EP_LINK_RE = /^hg:(\d{6,25}):(\d{1,5})(?:\|([^|]+)\|([^|]+))?$/;

function hgEpisodeLink(sid, no, conf) {
  return "hg:" + sid + ":" + no + "|" + encodeURIComponent(conf.server) + "|" + encodeURIComponent(conf.token);
}

// hgLinkConfig picks the config for an episode link: module settings passed
// in params, else the server/token in the link, else what a list stored.
// A link carrying a non-http(s) server yields null.
function hgLinkConfig(m, params) {
  if (params && params.server && params.token) return hgConfig(params);
  if (m[3]) {
    let server, token;
    try {
      server = decodeURIComponent(m[3]).replace(/\/+$/, "");
      token = decodeURIComponent(m[4]);
    } catch (e) {
      return null;
    }
    if (!/^https?:\/\/[^\s/]+/.test(server) || !token) return null;
    let quality = "1080p";
    try {
      const raw = Widget.storage.get(HG_CONF_KEY);
      const stored = raw && (typeof raw === "string" ? JSON.parse(raw) : raw);
      if (stored && stored.quality) quality = stored.quality;
    } catch (e) {}
    return { server, token, quality };
  }
  return hgConfig();
}

// hgEpisodeDetail is a single playable episode; rex resumes continue-watching
// entries by calling loadDetail with the episode's link.
async function hgEpisodeDetail(conf, sid, no, link) {
  try {
    const s = await hgGet(conf, "/api/series/" + sid);
    const e = (s.episodeList || []).find((x) => x.no === no);
    if (!e) return null;
    const item = hgItem(s, s.title + " 第" + no + "集");
    return Object.assign(item, {
      id: "hg:" + sid + ":" + no,
      link,
      episodeName: "第" + no + "集",
      duration: e.duration,
      videoUrl: e.url,
      playerType: "app",
    });
  } catch (error) {
    console.error(HG_TAG, "episode", link, error.message || error);
    throw error;
  }
}
const HG_QUALITIES = ["1080p", "720p", "480p"];

// loadResource returns play URLs for this module's episodes only. The link is
// trusted over id; anything not ours yields [] without a request.
async function loadResource(params = {}) {
  const raw = params.link != null && params.link !== "" ? params.link : params.id;
  const s = String(raw == null ? "" : raw);
  let sid, no;
  const m = HG_EP_LINK_RE.exec(s);
  let conf;
  if (m) {
    conf = hgLinkConfig(m, params);
    if (!conf) return [];
    sid = m[1];
    no = Number(m[2]);
  } else {
    const sm = HG_LINK_RE.exec(s);
    no = Number(params.episode);
    if (!sm || !(no > 0)) return [];
    sid = sm[1];
  }
  try {
    conf = conf || hgConfig(params);
    const order = [conf.quality, ...HG_QUALITIES.filter((q) => q !== conf.quality)];
    return order.map((q) => ({
      name: q.toUpperCase(),
      description: "红果 · hgsd 解密 · " + q.toUpperCase(),
      url: conf.server + "/play/" + sid + "/" + no + ".mp4?q=" + q + "&t=" + encodeURIComponent(conf.token),
      playerType: "app",
    }));
  } catch (error) {
    console.error(HG_TAG, "resource", s, error.message || error);
    throw error;
  }
}
