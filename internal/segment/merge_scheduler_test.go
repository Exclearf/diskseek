package segment

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
)

func TestRunMergeGroupsOrdersOutOfOrderResults(t *testing.T) {
	groups := []mergeGroup{{groupIndex: 0}, {groupIndex: 1}}
	secondFinished := make(chan struct{})
	var completionOrder []int

	got := runMergeGroups(groups, 2, func(group mergeGroup) (string, uint64, uint64, error) {
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
	groups := []mergeGroup{{groupIndex: 0}, {groupIndex: 1}, {groupIndex: 2}}
	started := make(chan int, len(groups))
	release := make(chan struct{})
	done := make(chan struct{})

	go func() {
		runMergeGroups(groups, 2, func(group mergeGroup) (string, uint64, uint64, error) {
			started <- group.groupIndex
			<-release
			return "", 0, 0, nil
		})
		close(done)
	}()

	<-started
	<-started
	select {
	case groupIndex := <-started:
		t.Errorf("group %d started above the worker limit", groupIndex)
	default:
	}
	close(release)
	<-done
}

func TestRunMergeGroupsStopsAfterConcurrentErrors(t *testing.T) {
	groups := []mergeGroup{{groupIndex: 0}, {groupIndex: 1}, {groupIndex: 2}}
	mergeErr := errors.New("merge failed")
	started := make(chan int, len(groups))
	release := make(chan struct{})
	done := make(chan []mergeGroupResult, 1)

	go func() {
		done <- runMergeGroups(groups, 2, func(group mergeGroup) (string, uint64, uint64, error) {
			started <- group.groupIndex
			<-release
			return "", 0, 0, mergeErr
		})
	}()

	startedGroups := map[int]bool{<-started: true, <-started: true}
	select {
	case <-done:
		close(release)
		t.Fatal("runMergeGroups returned while groups were active")
	default:
	}
	close(release)
	results := <-done
	close(started)
	for groupIndex := range started {
		startedGroups[groupIndex] = true
	}

	if len(startedGroups) != 2 || !startedGroups[0] || !startedGroups[1] {
		t.Fatalf("started groups = %v, want groups 0 and 1", startedGroups)
	}
	for groupIndex := range 2 {
		if !errors.Is(results[groupIndex].err, mergeErr) {
			t.Fatalf("result %d error = %v, want %v", groupIndex, results[groupIndex].err, mergeErr)
		}
	}
}
