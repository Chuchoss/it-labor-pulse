package normalize_test

import (
	"testing"

	"github.com/Chuchoss/it-labor-pulse/libs/go-common/normalize"
	"github.com/stretchr/testify/require"
)

func TestDetectRemote(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		schedule string
		formats  []string
		region   string
		title    string
		desc     string
		want     bool
	}{
		{name: "work_format_remote", formats: []string{"REMOTE"}, want: true},
		{name: "schedule_remote", schedule: "remote", want: true},
		{name: "region_remote", region: "Удалённо", want: true},
		{name: "keyword_only_title", title: "Remote Go Developer", want: false},
		{name: "keyword_title_and_desc", title: "Remote engineer", desc: "работа удалённо", want: true},
		{name: "office", schedule: "fullDay", formats: []string{"ON_SITE"}, region: "Москва", title: "Go", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := normalize.DetectRemote(tt.schedule, tt.formats, tt.region, tt.title, tt.desc)
			require.Equal(t, tt.want, got)
		})
	}
}
