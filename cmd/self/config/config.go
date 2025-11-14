// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package config

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func runConfig(_ *cobra.Command, _ []string) error {
	fmt.Println("Configuration initialized. Using config file:", viper.ConfigFileUsed())
	fmt.Println()

	// Print out the viper configuration for debugging
	// Alphabetically, and redacting sensitive information
	// TODO There has to be a better way of marking sensitive data
	// perhaps with leebenson/conform?
	keys := make([]string, 0, len(viper.AllSettings()))
	for key := range viper.AllSettings() {
		keys = append(keys, key)
	}

	sort.Strings(keys)

	for _, key := range keys {
		value := viper.Get(key)

		// TODO Skip token because its sensitive
		if key == "token" || key == "api_token" {
			fmt.Printf("  %s: %s\n", key, "****")
		} else {
			fmt.Printf("  %s: %v\n", key, value)
		}
	}

	return nil
}
