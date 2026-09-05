package segment

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"testing/synctest"
)

func TestRunMergeGroupsOrdersOutOfOrderResults(t *testing.T) {
	groups := []mergeGroup{{groupIndex: 0}, {groupIndex: 1}}
	secondFinished := make(chan struct{})
	var completionOrder []int

	got := runMergeGroups(context.Background(), groups, 2, func(_ context.Context, group mergeGroup) (string, uint64, uint64, error) {
		if group.groupIndex == 0 {
			<-secondFinished
		} else {
			completionOrder = append(completionOrder, group.groupIndex)
			close(secondFinished)
		}
		if group.groupIndex == 0 {
			completionOrder = append(completionOrder, group.groupIndex)
		}
		return fmt.Sprintf("g%d", group.groupIndex), uint64(group.groupIndex + 1), uint64(group.groupIndex + 2), nil
	})

	if !reflect.DeepEqual(completionOrder, []int{1, 0}) {
		t.Fatalf("completion order = %v, want [1 0]", completionOrder)
	}
	want := []mergeGroupResult{
		{groupIndex: 0, outputPath: "g0", inputBytes: 1, outputBytes: 2},
		{groupIndex: 1, outputPath: "g1", inputBytes: 2, outputBytes: 3},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("results = %#v, want %#v", got, want)
	}
}

func TestRunMergeGroupsLimitsConcurrentGroups(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		groups := []mergeGroup{{groupIndex: 0}, {groupIndex: 1}, {groupIndex: 2}}
		started := make(chan int, len(groups))
		release := make(chan struct{})
		done := make(chan struct{})

		go func() {
			runMergeGroups(context.Background(), groups, 2, func(_ context.Context, group mergeGroup) (string, uint64, uint64, error) {
				started <- group.groupIndex
				<-release
				return "", 0, 0, nil
			})
			close(done)
		}()

		<-started
		<-started
		synctest.Wait()
		select {
		case groupIndex := <-started:
			t.Errorf("group %d started above the worker limit", groupIndex)
		default:
		}
		close(release)
		<-done
	})
}

func TestRunMergeGroupsCancelsActiveSiblingAfterError(t *testing.T) {
	groups := []mergeGroup{{groupIndex: 0}, {groupIndex: 1}, {groupIndex: 2}}
	mergeErr := errors.New("merge failed")
	siblingStarted := make(chan struct{})
	var started [3]bool

	results := runMergeGroups(context.Background(), groups, 2, func(ctx context.Context, group mergeGroup) (string, uint64, uint64, error) {
		started[group.groupIndex] = true
		switch group.groupIndex {
		case 0:
			<-siblingStarted
			return "", 0, 0, mergeErr
		case 1:
			close(siblingStarted)
			<-ctx.Done()
			return "", 0, 0, ctx.Err()
		default:
			return "", 0, 0, nil
		}
	})
	if !started[0] || !started[1] || started[2] {
		t.Fatalf("started groups = %v, want [true true false]", started)
	}
	if !errors.Is(results[0].err, mergeErr) {
		t.Fatalf("result 0 error = %v, want %v", results[0].err, mergeErr)
	}
	if !errors.Is(results[1].err, context.Canceled) {
		t.Fatalf("result 1 error = %v, want %v", results[1].err, context.Canceled)
	}
}
