// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build kubelet,kubeapiserver

package hostinfo

import (
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/tagger/utils"
)

// GetTags gets the tags from the kubernetes apiserver
func GetTags() ([]string, error) {
	labelsToTags := config.Datadog.GetStringMapString("kubernetes_node_labels_as_tags")

	if len(labelsToTags) == 0 {
		// Nothing to extract
		return nil, nil
	}

	// viper lower-cases map keys from yaml, but not from envvars
	for label, value := range labelsToTags {
		delete(labelsToTags, label)
		labelsToTags[strings.ToLower(label)] = value
	}

	nodeLabels, err := GetNodeLabels()
	if err != nil {
		return nil, err
	}

	return extractTags(nodeLabels, labelsToTags), nil
}

func extractTags(nodeLabels, labelsToTags map[string]string) []string {
	tagList := utils.NewTagList()

	for labelName, labelValue := range nodeLabels {
		if tagName, found := labelsToTags[strings.ToLower(labelName)]; found {
			tagList.AddLow(tagName, labelValue)
		}
	}

	tags, _, _ := tagList.Compute()
	return tags
}
