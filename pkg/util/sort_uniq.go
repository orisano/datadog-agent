// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package util

import "sort"

const insertionSortThreshold = 20

// SortUniqInPlace sorts and remove duplicates from elements in place
// The returned slice is a subslice of elements
func SortUniqInPlace(elements []string) []string {
	if len(elements) < 2 {
		return elements
	}
	size := len(elements)
	if size <= insertionSortThreshold {
		insertionSort(elements)
	} else {
		// this will trigger an alloc because sorts uses interface{} internaly
		// which confuses the escape analysis
		sort.Strings(elements)
	}
	return uniqSorted(elements)
}

func insertionSort(elements []string) {
	for i := 1; i < len(elements); i++ {
		temp := elements[i]
		j := i
		for j > 0 && temp <= elements[j-1] {
			elements[j] = elements[j-1]
			j--
		}
		elements[j] = temp
	}
}

func uniqSorted(elements []string) []string {
	j := 0
	for i := 1; i < len(elements); i++ {
		if elements[j] == elements[i] {
			continue
		}
		j++
		elements[j] = elements[i]
	}
	return elements[:j+1]
}
