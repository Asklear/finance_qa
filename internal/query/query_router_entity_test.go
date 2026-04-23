package query

import "testing"

func TestIsRealishQueryEntityRejectsSyntheticTemporalAndGenericFragments(t *testing.T) {
	cases := []struct {
		name   string
		entity string
		want   bool
	}{
		{name: "real business entity", entity: "飞未云科", want: true},
		{name: "empty", entity: "", want: false},
		{name: "generic metric", entity: "收入", want: false},
		{name: "temporal fragment", entity: "Q1", want: false},
		{name: "synthetic question fragment", entity: "单笔最大流入来自谁", want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isRealishQueryEntity(tc.entity); got != tc.want {
				t.Fatalf("isRealishQueryEntity(%q) = %t, want %t", tc.entity, got, tc.want)
			}
		})
	}
}
