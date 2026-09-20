package quota

import (
	"context"
	"encoding/json"
	"testing"
	"cpa-usage-keeper/internal/cpa/dto/apicall"
	"cpa-usage-keeper/internal/entities"
)

type qoderTestCaller struct { body string }
func (caller qoderTestCaller) CallManagementAPI(context.Context, apicall.Request) (*apicall.Response, error) { panic("must use plugin credits endpoint") }
func (caller qoderTestCaller) FetchQoderCredits(context.Context, string) ([]byte, error) { return []byte(caller.body), nil }

func TestQoderCredits(t *testing.T) {
	caller := qoderTestCaller{body: `{"accounts":[{"auth_index":"other","error":"wrong account"},{"auth_index":"test","credits":{"packages":[{"kind":"plan","name":"Teams","remain":0,"used":1000,"size":1000,"size_known":true,"available":true},{"kind":"dedicated","name":"SOTA","remain":19.5,"used":0.5,"size":20,"size_known":true,"available":true,"cycle_end":"2000-01-01T00:00:00Z"},{"kind":"shared","name":"Shared","remain":0,"used":0,"size":-1,"size_known":false,"available":false}]}}]}`}
	provider := qoderProvider{caller: upstreamResponseRecordingCaller{caller: caller}}
	output, err := provider.Check(context.Background(), ProviderInput{Identity: entities.UsageIdentity{Identity: "test"}})
	if err != nil { t.Fatal(err) }
	rows := NormalizeQuotaRows(output)
	if len(rows) != 3 || *rows[0].Remaining != 0 || *rows[1].Remaining != 19.5 || *rows[1].Allowed || rows[1].ResetAt != "" || rows[1].ExpiresAt == "" || rows[2].Limit != nil || rows[2].UsedPercent != nil { t.Fatalf("unexpected rows: %+v", rows) }
	for _, body := range []string{`{"accounts":[]}`, `{"accounts":[{"auth_index":"test","error":"failed"}]}`, `{"accounts":[{"auth_index":"test","credits":{"packages":[{"name":"missing values"}]}}]}`} {
		_, err := (qoderProvider{caller: qoderTestCaller{body: body}}).Check(context.Background(), ProviderInput{Identity: entities.UsageIdentity{Identity: "test"}})
		if err == nil { t.Fatalf("accepted invalid payload: %s", body) }
	}
	response, err := (&Service{}).Refresh(context.Background(), RefreshRequest{AuthIndexes: []string{""}})
	if err != nil { t.Fatal(err) }
	encoded, err := json.Marshal(response)
	if err != nil { t.Fatal(err) }
	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil { t.Fatal(err) }
	if _, ok := decoded["tasks"].([]any); !ok { t.Fatalf("tasks must be array: %s", encoded) }
}
