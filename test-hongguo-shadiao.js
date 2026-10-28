// node test-hongguo-shadiao.js [widget path]  — offline test with a mocked Widget
const fs = require("fs");
const assert = require("assert/strict");

const SERVER = "http://hg.test:8686";
const TOKEN = "tok";
const base = { server: SERVER, token: TOKEN, quality: "720p" };

const show = (id, title, extra = {}) => ({
  id, title, description: "d" + id, cover: "https://img/" + id + ".heic", episodes: 3,
  finished: true, heat: 10, created: 1752537600, tags: ["搞笑", "古代"], ...extra,
});

const calls = [];
global.Widget = {
  http: {
    get: async (url, opts = {}) => {
      calls.push({ url, params: opts.params || {} });
      if (url === SERVER + "/api/list") return { data: [show("7527216662647426110", "坑爹悍将")] };
      if (url === SERVER + "/api/search") return { data: [show("7000000000000000002", "沙雕乙")] };
      if (url === SERVER + "/api/history") return {
        data: [
          show("7000000000000000001", "甲", { lastEpisode: 3, completed: true }),
          show("7000000000000000002", "乙", { lastEpisode: 2, caughtUp: true, hasUpdate: true }),
          show("7000000000000000003", "丙", { lastEpisode: 1 }),
        ],
      };
      if (url === SERVER + "/api/series/7699740362100706328") return {
        data: {
          ...show("7699740362100706328", "荒岛", { lastEpisode: 2 }),
          episodeList: [1, 2, 3].map((n) => ({ no: n, title: "", duration: 60, url: `${SERVER}/play/7699740362100706328/${n}.mp4?q=720p&t=${TOKEN}` })),
          related: [show("7527216662647426110", "坑爹悍将")],
        },
      };
      if (url === SERVER + "/api/series/7000000000000000009") return {
        data: { ...show("7000000000000000009", "没看过", { episodes: 609 }), episodeList: [{ no: 1, title: "", duration: 60, url: "u1" }], related: [] },
      };
      throw new Error("unmocked url: " + url);
    },
  },
  storage: { _m: {}, get(k) { return this._m[k]; }, set(k, v) { this._m[k] = v; }, remove(k) { delete this._m[k]; } },
};
global.WidgetMetadata = {};
eval(fs.readFileSync(process.argv[2] || "./widgets/hongguo-shadiao.js", "utf8"));

(async () => {
  // metadata
  assert.equal(WidgetMetadata.id, "xufeng.hongguo.shadiao");
  assert.equal(WidgetMetadata.title, "沙雕漫剧");

  // the subscription index must point at this widget and carry the same version
  const fwd = JSON.parse(fs.readFileSync("./shadiao-rex.fwd", "utf8"));
  const entry = fwd.widgets.find((w) => w.id === WidgetMetadata.id);
  assert.ok(entry, "widget listed in shadiao-rex.fwd");
  assert.equal(entry.version, WidgetMetadata.version);
  assert.equal(entry.url, "https://raw.githubusercontent.com/xxxxxufeng/shadiao-rex/main/widgets/hongguo-shadiao.js");
  const gp = WidgetMetadata.globalParams.map((p) => p.name);
  assert.deepEqual(gp, ["server", "token", "quality"]);
  const cats = ["funny", "xianxia", "ancient", "urban", "horror", "system"];
  assert.deepEqual(WidgetMetadata.modules.map((m) => m.functionName),
    ["loadHot", "loadNew", ...cats.map(() => "loadCategory"), "loadHistory", "loadResource"]);
  WidgetMetadata.modules.filter((m) => m.functionName === "loadCategory").forEach((m, i) => {
    assert.equal(m.id, "cat_" + cats[i]);
    assert.ok(m.title.startsWith("沙雕·"));
    const c = m.params.find((p) => p.name === "cat");
    assert.equal(c.type, "constant");
    assert.equal(c.value, cats[i]);
    assert.ok(m.params.some((p) => p.type === "page"));
  });
  assert.ok(!WidgetMetadata.modules.some((m) => m.functionName === "searchList"));
  assert.equal(WidgetMetadata.search.functionName, "search");
  assert.ok(WidgetMetadata.search.params.some((p) => p.type === "page"));
  assert.ok(!WidgetMetadata.modules.some((m) => m.type === "search"));

  // list
  const hot = await loadHot({ ...base, page: 2 });
  assert.equal(calls.at(-1).url, SERVER + "/api/list");
  assert.deepEqual(calls.at(-1).params, { kind: "hot", page: 2, t: TOKEN, q: "720p" });
  const it = hot[0];
  assert.equal(it.type, "url");
  assert.equal(it.id, "hg:7527216662647426110");
  assert.equal(it.link, "hg:7527216662647426110");
  assert.equal(it.mediaType, "tv");
  assert.equal(it.coverUrl, "https://img/7527216662647426110.heic");
  assert.equal(it.posterPath, it.coverUrl);
  assert.equal(it.releaseDate, "2025-07-15");
  assert.deepEqual(it.genreItems, [{ id: "搞笑", title: "搞笑" }, { id: "古代", title: "古代" }]);
  assert.equal(it.cover, undefined);
  // rex reads `episode` as the episode to play, so list items must not carry the total count
  assert.equal(it.episode, undefined);
  assert.equal(it.stills, undefined);

  await loadNew({ ...base });
  assert.equal(calls.at(-1).params.kind, "new");
  assert.equal(calls.at(-1).params.page, 1);

  // category & tag (genreItems tap reopens a list module with genreId)
  await loadCategory({ ...base, cat: "horror", sort: "new", page: 3 });
  assert.equal(calls.at(-1).url, SERVER + "/api/list");
  assert.deepEqual(calls.at(-1).params, { kind: "new", cat: "horror", page: 3, t: TOKEN, q: "720p" });
  await loadHot({ ...base, genreId: "规则怪谈" });
  assert.deepEqual(calls.at(-1).params, { kind: "hot", tag: "规则怪谈", page: 1, t: TOKEN, q: "720p" });

  // detail
  const d = await loadDetail("hg:7699740362100706328");
  // loadDetail gets only the link; config comes from what list handlers stored
  assert.deepEqual(calls.at(-1).params, { t: TOKEN, q: "720p" });
  assert.equal(d.link, "hg:7699740362100706328");
  assert.equal(d.episodeItems.length, 3);
  assert.equal(d.episode, 2, "play button resumes at the last watched episode");
  const fresh = await loadDetail("hg:7000000000000000009");
  assert.equal(fresh.episode, 1, "unwatched series starts at episode 1, not the latest");
  assert.equal(d.episodeItems[1].title, "▶ 第2集（上次看到）");
  assert.equal(d.episodeItems[0].title, "第1集");
  assert.equal(d.episodeItems[2].videoUrl, `${SERVER}/play/7699740362100706328/3.mp4?q=720p&t=${TOKEN}`);
  // rex defaults the play button to the highest `episode`; episodes carry no number field
  assert.equal(d.episodeItems[2].episode, undefined);
  assert.equal(d.episodeItems[2].episodeName, "第3集");
  assert.equal(d.episodeItems[2].playerType, "app");
  assert.ok(Array.isArray(d.relatedItems) && d.relatedItems[0].link.startsWith("hg:"));
  assert.equal(d.recommendations, undefined);
  assert.equal(d.childItems, undefined);

  // episodes carry a link so rex can resume them later through loadResource
  // the link carries server+token: rex drops Widget.storage on relaunch
  const EP3 = `hg:7699740362100706328:3|${encodeURIComponent(SERVER)}|${TOKEN}`;
  assert.equal(d.episodeItems[2].link, EP3);
  assert.equal(d.episodeItems[2].id, "hg:7699740362100706328:3");

  // loadResource: rex's "continue watching" re-resolves the episode through this module
  const rm = WidgetMetadata.modules.find((m) => m.id === "loadResource");
  assert.ok(rm && rm.type === "stream" && rm.functionName === "loadResource");
  let n0 = calls.length;
  const res = await loadResource({ link: "hg:7699740362100706328:31" });
  assert.equal(calls.length, n0, "builds play URLs locally, no request");
  assert.deepEqual(res.map((r) => r.name), ["720P", "1080P", "480P"], "configured quality first");
  assert.equal(res[0].url, `${SERVER}/play/7699740362100706328/31.mp4?q=720p&t=${TOKEN}`);
  assert.ok(res.every((r) => r.playerType === "app"));
  // series-level link + episode param also resolves
  const res2 = await loadResource({ link: "hg:7699740362100706328", episode: 5 });
  assert.equal(res2[0].url, `${SERVER}/play/7699740362100706328/5.mp4?q=720p&t=${TOKEN}`);
  // id is used when there is no link
  assert.equal((await loadResource({ id: "hg:7699740362100706328:2" })).length, 3);
  // foreign / malformed inputs: nothing, and no requests
  for (const bad of [{ id: 550 }, { tmdbId: 550, type: "movie" }, { link: "rou:1:2" }, { link: "https://evil.test/hg:1:2" },
    { link: "hg:7699740362100706328" }, { link: "hg:abc:1" }, { link: "bad", id: "hg:7699740362100706328:2" }, {}]) {
    assert.deepEqual(await loadResource(bad), [], JSON.stringify(bad));
  }
  assert.equal(calls.length, n0);

  // continue-watching after an app relaunch: storage is empty, the link has what we need
  const saved = Widget.storage._m;
  Widget.storage._m = {};
  const ep = await loadDetail(EP3);
  assert.equal(calls.at(-1).url, SERVER + "/api/series/7699740362100706328");
  assert.equal(ep.link, EP3);
  assert.equal(ep.id, "hg:7699740362100706328:3");
  assert.equal(ep.type, "url");
  assert.equal(ep.videoUrl, `${SERVER}/play/7699740362100706328/3.mp4?q=720p&t=${TOKEN}`);
  assert.equal(ep.title, "荒岛 第3集");
  assert.equal(ep.episodeName, "第3集");
  assert.equal(ep.coverUrl, "https://img/7699740362100706328.heic");
  assert.equal(ep.episode, undefined);
  const rl = await loadResource({ link: EP3 });
  assert.equal(rl[0].url, `${SERVER}/play/7699740362100706328/3.mp4?q=1080p&t=${TOKEN}`, "no stored quality: 1080p first");
  // globals passed to loadDetail as a second argument also work
  assert.equal((await loadDetail("hg:7699740362100706328:3", base)).link, "hg:7699740362100706328:3");
  // a link carrying a non-http server is rejected without a request
  const nb = calls.length;
  assert.equal(await loadDetail(`hg:7699740362100706328:3|${encodeURIComponent("javascript:x")}|${TOKEN}`), null);
  assert.deepEqual(await loadResource({ link: `hg:7699740362100706328:3|${encodeURIComponent("file:///etc")}|t` }), []);
  assert.equal(calls.length, nb);
  Widget.storage._m = saved;
  // an episode number the series doesn't have
  assert.equal(await loadDetail("hg:7699740362100706328:99"), null);

  // detail rejects foreign links with zero requests
  const before = calls.length;
  for (const bad of ["550", "rou:123", "https://evil.test/hg:1", "", "hg:", "hg:abc", "hg:123:4", "hg:7699740362100706328:x", null, undefined]) {
    assert.equal(await loadDetail(bad), null, String(bad));
  }
  assert.equal(calls.length, before);

  // search
  const sr = await search({ ...base, keyword: "沙雕", page: 1 });
  assert.equal(calls.at(-1).url, SERVER + "/api/search");
  assert.equal(calls.at(-1).params.q, "沙雕");
  assert.equal(sr[0].link, "hg:7000000000000000002");

  // history labels
  const h = await loadHistory({ ...base });
  assert.equal(h[0].title, "甲 · 已看完");
  assert.equal(h[1].title, "乙 · 有更新");
  assert.equal(h[2].title, "丙 · 看到第1集");

  // missing config: clear error, no request
  Widget.storage._m = {};
  const n = calls.length;
  await assert.rejects(loadHot({ page: 1 }), /填写 server 和 token/);
  await assert.rejects(loadDetail("hg:7699740362100706328"), /填写 server 和 token/);
  assert.equal(calls.length, n);

  console.log("✅ ok", calls.length, "calls");
})().catch((e) => { console.error("❌", e); process.exit(1); });
