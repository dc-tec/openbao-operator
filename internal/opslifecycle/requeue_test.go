package opslifecycle

import (
	"testing"

	"github.com/dc-tec/openbao-operator/internal/constants"
)

func TestRequeueDelay(t *testing.T) {
	t.Parallel()

	cases := map[RetryClass]int64{
		RetryClassLockContention: int64(constants.RequeueShort),
		RetryClassProgressPoll:   int64(constants.RequeueShort),
		RetryClassStandard:       int64(constants.RequeueStandard),
		RetryClass("unknown"):    int64(constants.RequeueStandard),
	}

	for class, expected := range cases {
		class := class
		expected := expected
		t.Run(string(class), func(t *testing.T) {
			t.Parallel()
			got := RequeueDelay(class)
			if int64(got) != expected {
				t.Fatalf("expected requeue delay %d for %s, got %d", expected, class, got)
			}
		})
	}
}
