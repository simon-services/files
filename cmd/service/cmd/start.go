/*
Copyright © 2020 NAME HERE <EMAIL ADDRESS>

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package cmd

import (
	"fmt"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"simon.services/files"
)

var (
	cfgFile string
	address string
	client  string
	secret  string
	webroot string
	err     error
)

// startCmd represents the start command
var startCmd = &cobra.Command{
	Use:   "start",
	Short: "starts the files service",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(cfgFile) > 0 {
			address = viper.GetString("address")
			client = viper.GetString("client")
			secret = viper.GetString("secret")
			webroot = viper.GetString("webroot")
		} else {
			address, err = cmd.Flags().GetString("address")
			if err != nil {
				return err
			}
			client, err = cmd.Flags().GetString("client")
			if err != nil {
				return err
			}
			secret, err = cmd.Flags().GetString("secret")
			if err != nil {
				return err
			}
			webroot, err = cmd.Flags().GetString("webroot")
			if err != nil {
				return err
			}
		}
		f := files.New()
		return f.Start(address, client, secret, webroot)
	},
}

func init() {
	cobra.OnInitialize(initConfig)
	rootCmd.AddCommand(startCmd)
	startCmd.Flags().StringVar(&cfgFile, "config", "", "config file (default is /opt/simon.services/files/conf/files.json)")
	startCmd.Flags().StringVar(&address, "address", "0.0.0.0:7878", "address to start the service at)")
	startCmd.Flags().StringVar(&client, "client", "files", "client ID")
	startCmd.Flags().StringVar(&secret, "secret", "secret", "client secret")
	startCmd.Flags().StringVar(&webroot, "webroot", "./webroot", "location of the frontend static files")
}

func initConfig() {
	viper.SetConfigType("json")
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	}
	viper.AddConfigPath("/opt/simon.services/files/conf")
	viper.SetConfigName("files.json")
	viper.AutomaticEnv()
	if err := viper.ReadInConfig(); err == nil {
		fmt.Println("Using config file:", viper.ConfigFileUsed())
	}
}
