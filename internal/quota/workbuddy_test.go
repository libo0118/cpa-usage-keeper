package quota

import (
	"context"
	"cpa-usage-keeper/internal/entities"
	"testing"
)

type workbuddyTestCaller struct{ qoderTestCaller }

func (c workbuddyTestCaller) FetchWorkBuddyCredits(context.Context, string) ([]byte, error) {
	return []byte(c.body), nil
}

func TestWorkBuddyCredits(t *testing.T) {
	body := `{"accounts":[{"auth_index":"other","error":"wrong"},{"auth_index":"test","region":"cn","credits":{"total_remain":4,"total_used":6,"total_size":10,"packages":[{"name":"Pack","remain":4,"used":1,"size":5,"cycle_end":"2099-09-30 23:59:59"},{"name":"Pack","remain":0,"used":5,"size":5,"cycle_end":"2000-09-30 23:59:59"}]}}]}`
	input := ProviderInput{Identity: entities.UsageIdentity{Identity: "test"}}
	check := func(body string) (ProviderOutput, error) {
		return (workbuddyProvider{caller: upstreamResponseRecordingCaller{ManagementClient: workbuddyTestCaller{qoderTestCaller{body: body}}}}).Check(context.Background(), input)
	}
	output, err := check(body)
	if err != nil {
		t.Fatal(err)
	}
	rows := NormalizeQuotaRows(output)
	if len(rows) != 3 || *rows[0].Remaining != 4 || rows[1].Key == rows[2].Key || !*rows[1].Allowed || *rows[2].Allowed || rows[1].ExpiresAt != "2099-09-30T23:59:59+08:00" {
		t.Fatalf("incorrect rows: %+v", rows)
	}
	for _, bad := range []string{`{"accounts":[]}`, `{"accounts":[{"auth_index":"test","error":"expired"}]}`, `{"accounts":[{"auth_index":"test","credits":{"total_remain":0}}]}`, `{"accounts":[{"auth_index":"test","credits":{"total_remain":-1,"total_used":0,"total_size":0}}]}`} {
		if _, err := check(bad); err == nil {
			t.Fatal("accepted invalid account quota")
		}
	}
}
