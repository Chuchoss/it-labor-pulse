package normalize_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Chuchoss/it-labor-pulse/libs/go-common/normalize"
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
		want     *bool
	}{
		{name: "work_format_remote", formats: []string{"REMOTE"}, want: boolPtr(true)},
		{name: "schedule_remote", schedule: "remote", want: boolPtr(true)},
		{name: "text_is_not_authoritative", region: "Удалённо", title: "Remote", desc: "удалённо", want: nil},
		{name: "office", schedule: "fullDay", formats: []string{"ON_SITE"}, region: "Москва", title: "Go", want: boolPtr(false)},
		{name: "unknown", want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := normalize.DetectRemote(tt.schedule, tt.formats, tt.region, tt.title, tt.desc)
			require.Equal(t, tt.want, got)
		})
	}
}

func boolPtr(value bool) *bool {
	return &value
}
