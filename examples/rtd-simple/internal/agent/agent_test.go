// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"context"
	"testing"

	pb "github.com/indexexchange/rtd-simple/pkg/pb/artf"
	openrtb "github.com/indexexchange/rtd-simple/pkg/pb/openrtb"
)

func s(v string) *string { return &v }

func req(id string, lc pb.Lifecycle, withResp bool) *pb.RTBRequest {
	r := &pb.RTBRequest{
		Lifecycle: &lc,
		Id:        s(id),
		BidRequest: &openrtb.BidRequest{
			Id:                       s("brq-" + id),
			DistributionchannelOneof: &openrtb.BidRequest_Site_{Site: &openrtb.BidRequest_Site{Domain: s("example.com")}},
			Imp:                      []*openrtb.BidRequest_Imp{{Id: s("imp-1")}},
		},
	}
	if withResp {
		r.BidResponse = &openrtb.BidResponse{Id: s("bresp-" + id)}
	}
	return r
}

// TestNoOpReturnsEmptyMutations proves the container never mutates, on either
// lifecycle, while still returning a valid response (id echoed, api version set).
func TestNoOpReturnsEmptyMutations(t *testing.T) {
	a := New("9.9.9", 1)
	cases := []struct {
		name     string
		r        *pb.RTBRequest
	}{
		{"publisher request", req("pub", lifecyclePublisherBidRequest, false)},
		{"dsp bid response", req("dsp", lifecycleDSPBidResponse, true)},
		{"unspecified lifecycle", req("uns", 0, false)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := a.GetMutations(context.Background(), tc.r)
			if err != nil {
				t.Fatalf("GetMutations error: %v", err)
			}
			if len(resp.GetMutations()) != 0 {
				t.Errorf("expected 0 mutations, got %d", len(resp.GetMutations()))
			}
			if resp.GetId() != tc.r.GetId() {
				t.Errorf("id not echoed: got %q want %q", resp.GetId(), tc.r.GetId())
			}
			if resp.GetMetadata().GetApiVersion() != "9.9.9" {
				t.Errorf("api version not set: got %q", resp.GetMetadata().GetApiVersion())
			}
		})
	}
}

// TestLifecycleName covers the log-label mapping.
func TestLifecycleName(t *testing.T) {
	if got := lifecycleName(lifecyclePublisherBidRequest); got != "PUBLISHER_BID_REQUEST" {
		t.Errorf("publisher: %q", got)
	}
	if got := lifecycleName(lifecycleDSPBidResponse); got != "DSP_BID_RESPONSE" {
		t.Errorf("dsp: %q", got)
	}
	if got := lifecycleName(0); got != "UNSPECIFIED" {
		t.Errorf("unspecified: %q", got)
	}
}
