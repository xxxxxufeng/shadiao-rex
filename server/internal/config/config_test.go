package config

import (
	"reflect"
	"testing"
)

func TestLoadCreatesDefaults(t *testing.T) {
	dir := t.TempDir()
	a, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Token) != 64 {
		t.Fatalf("token len %d", len(a.Token))
	}
	want := []string{"沙雕", "表情包", "沙雕动画", "搞笑", "无厘头", "爆笑", "穿越", "系统", "修仙", "逆袭", "规则怪谈", "诡异", "末世", "无限流", "游戏", "异能", "升级", "古代", "都市", "校园", "年代", "乡村", "直播", "求生", "玄幻", "重生", "神豪", "皇帝", "大明", "三国", "西游", "熊猫头", "怪谈", "宿舍", "猎人", "家族", "仙门", "师兄", "老六", "整活"}
	if !reflect.DeepEqual(a.Keywords, want) {
		t.Fatalf("keywords %v", a.Keywords)
	}
	if a.Listen != ":8686" || a.RefreshHours != 6 || a.DataDir != dir {
		t.Fatalf("defaults %+v", a)
	}
	b, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if b.Token != a.Token {
		t.Fatal("token changed across loads")
	}
}
