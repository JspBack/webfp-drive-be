package store

import "testing"

func TestStoragePlanLimits(t *testing.T) {
	const mib = 1024 * 1024
	fs := &FileStore{storageLimit: 500 * mib}
	for _, test := range []struct {
		plan string
		want int64
	}{
		{plan: "free", want: 500 * mib},
		{plan: "pro", want: 1024 * mib},
		{plan: "admin", want: 0},
	} {
		t.Run(test.plan, func(t *testing.T) {
			got, err := fs.storageLimitForPlan(test.plan)
			if err != nil || got != test.want {
				t.Errorf("storageLimitForPlan(%q) = %d, %v; want %d, nil", test.plan, got, err, test.want)
			}
		})
	}

	if _, err := fs.storageLimitForPlan("unknown"); err == nil {
		t.Error("storageLimitForPlan accepted an unknown plan")
	}
}
