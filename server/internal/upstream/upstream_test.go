package upstream

import (
	"reflect"
	"testing"
	"time"

	"github.com/beilunyang/openhongguo/sdk/model"
)

func TestToShow(t *testing.T) {
	s := &model.Series{
		SeriesSummary: model.SeriesSummary{ID: "7527216662647426110", Title: "坑爹悍将", Description: "d", CoverURL: "c", EpisodeCount: 609},
		Status:        model.SeriesEnd,
		Heat:          41575684,
		CreatedAt:     time.Unix(1752537600, 0),
		Rows: []model.InfoRow{
			{Kind: model.InfoCategory, Text: "无厘头"},
			{Kind: model.InfoSeasonOrder, Text: "第1季"},
			{Kind: model.InfoCategory, Text: "搞笑"},
		},
	}
	got := toShow(s)
	want := Show{ID: "7527216662647426110", Title: "坑爹悍将", Description: "d", Cover: "c", Episodes: 609,
		Finished: true, Heat: 41575684, Created: time.Unix(1752537600, 0), Tags: []string{"无厘头", "搞笑"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v", got)
	}
	s.Status = model.SeriesUpdating
	if toShow(s).Finished {
		t.Fatal("updating series marked finished")
	}
}
