package util

import (
	"reflect"
	"testing"
)

func TestRemoveDuplicates_String(t *testing.T) {
	input := []string{"a", "b", "a", "c", "b", "d"}
	want := []string{"a", "b", "c", "d"}
	got := RemoveDuplicates(input)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("RemoveDuplicates(strings) = %v, want %v", got, want)
	}
}

func TestRemoveDuplicates_Int(t *testing.T) {
	input := []int{1, 2, 2, 3, 1, 4}
	want := []int{1, 2, 3, 4}
	got := RemoveDuplicates(input)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("RemoveDuplicates(ints) = %v, want %v", got, want)
	}
}

func TestRemoveEmpty_String(t *testing.T) {
	input := []string{"a", "", "b", "", "c"}
	want := []string{"a", "b", "c"}
	got := RemoveEmpty(input)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("RemoveEmpty(strings) = %v, want %v", got, want)
	}
}

func TestRemoveEmpty_Int(t *testing.T) {
	input := []int{0, 1, 2, 0, 3}
	want := []int{1, 2, 3}
	got := RemoveEmpty(input)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("RemoveEmpty(ints) = %v, want %v", got, want)
	}
}

func TestRemoveDuplicates_AlreadyUnique(t *testing.T) {
	input := []string{"x", "y", "z"}
	want := []string{"x", "y", "z"}
	got := RemoveDuplicates(input)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("RemoveDuplicates(already unique) = %v, want %v", got, want)
	}
}

func TestRemoveEmpty_NoEmpty(t *testing.T) {
	input := []int{1, 2, 3}
	want := []int{1, 2, 3}
	got := RemoveEmpty(input)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("RemoveEmpty(no empty) = %v, want %v", got, want)
	}
}
