package util

func RemoveDuplicates[T comparable](slice []T) []T {
	uniqueMap := make(map[T]bool)
	uniqueSlice := []T{}
	for _, item := range slice {
		if !uniqueMap[item] {
			uniqueMap[item] = true
			uniqueSlice = append(uniqueSlice, item)
		}
	}
	return uniqueSlice
}

func RemoveEmpty[T comparable](slice []T) []T {
	var zero T
	nonEmptySlice := []T{}
	for _, item := range slice {
		if item != zero {
			nonEmptySlice = append(nonEmptySlice, item)
		}
	}
	return nonEmptySlice
}
