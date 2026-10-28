package pool

import "github.com/xxxxxufeng/shadiao-rex/server/internal/upstream"

type Category struct {
	ID, Title string
	Tags      []string
}

// Categories group the pool's theme tags (both tag taxonomies) for browsing.
// The rex module lists the same IDs and titles.
var Categories = []Category{
	{"funny", "搞笑", []string{"搞笑", "轻松搞笑", "喜剧", "无厘头"}},
	{"xianxia", "修仙玄幻", []string{"玄幻", "玄幻仙侠", "修真", "修仙问道", "降妖除魔"}},
	{"ancient", "古代宫廷", []string{"古代", "古装", "宫廷", "历史", "古装传奇", "乱世"}},
	{"urban", "都市日常", []string{"都市", "都市日常", "校园", "年代", "乡村", "年代创业", "总裁"}},
	{"horror", "恐怖怪谈", []string{"规则怪谈", "单元怪谈", "诡秘冒险", "高能生存", "无限流", "副本闯关", "冒险求生", "末世", "悬疑", "惊悚"}},
	{"system", "系统升级", []string{"系统", "升级流", "游戏世界"}},
}

// Match returns a predicate for a category ID and/or an exact tag; both empty
// matches everything, an unknown category matches nothing.
func Match(category, tag string) func(upstream.Show) bool {
	var want map[string]bool
	if category != "" {
		want = map[string]bool{}
		for _, c := range Categories {
			if c.ID == category {
				for _, t := range c.Tags {
					want[t] = true
				}
			}
		}
	}
	return func(s upstream.Show) bool {
		if want == nil && tag == "" {
			return true
		}
		for _, t := range s.Tags {
			if (want != nil && want[t]) || (tag != "" && t == tag) {
				return true
			}
		}
		return false
	}
}
