package opslifecycle

import (
	"testing"
)

func TestRequeueDelay(t *testing.T) {
	t.Parallel()

	cases := map[RetryClass]int64{
		RetryClassLockContention: int64(requeueShort),
		RetryClassProgressPoll:   int64(requeueShort),
		RetryClassStandard:       int64(requeueStandard),
		RetryClass("unknown"):    int64(requeueStandard),
	}

	for class, expected := range cases {
		t.Run(string(class), func(t *testing.T) {
			t.Parallel()
			got := RequeueDelay(class)
			if int64(got) != expected {
				t.Fatalf("expected requeue delay %d for %s, got %d", expected, class, got)
			}
		})
	}
}
