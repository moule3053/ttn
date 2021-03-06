// Copyright © 2016 The Things Network
// Use of this source code is governed by the MIT license that can be found in the LICENSE file.

package cmd

import (
	"strconv"
	"time"

	"github.com/TheThingsNetwork/ttn/api/router"
	"github.com/TheThingsNetwork/ttn/core/types"
	"github.com/TheThingsNetwork/ttn/ttnctl/util"
	"github.com/apex/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var uplinkCmd = &cobra.Command{
	Hidden: true,
	Use:    "uplink [DevAddr] [NwkSKey] [AppSKey] [FCnt] [Payload]",
	Short:  "Simulate an uplink message to the network",
	Long:   `ttnctl uplink simulates an uplink message to the network`,
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) < 5 {
			cmd.UsageFunc()(cmd)
			return
		}

		devAddr, err := types.ParseDevAddr(args[0])
		if err != nil {
			ctx.WithError(err).Fatal("Invalid DevAddr")
		}

		nwkSKey, err := types.ParseNwkSKey(args[1])
		if err != nil {
			ctx.WithError(err).Fatal("Invalid NwkSKey")
		}

		appSKey, err := types.ParseAppSKey(args[2])
		if err != nil {
			ctx.WithError(err).Fatal("Invalid AppSKey")
		}

		fCnt, err := strconv.Atoi(args[3])
		if err != nil {
			ctx.WithError(err).Fatal("Invalid FCnt")
		}

		payload, err := types.ParseHEX(args[4], len(args[4])/2)
		if err != nil {
			ctx.WithError(err).Fatal("Invalid Payload")
		}

		withDownlink, _ := cmd.Flags().GetBool("downlink")

		confirmed, _ := cmd.Flags().GetBool("confirmed")
		if confirmed {
			withDownlink = true
		}

		rtrClient := util.GetRouter(ctx)
		defer rtrClient.Close()

		gatewayID := viper.GetString("gateway-id")
		gatewayToken := viper.GetString("gateway-token")

		if gatewayID != "dev" {
			account := util.GetAccount(ctx)
			token, err := account.GetGatewayToken(gatewayID)
			if err != nil {
				ctx.WithError(err).Warn("Could not get gateway token")
				ctx.Warn("Trying without token. Your message may not be processed by the router")
				gatewayToken = ""
			} else if token != nil && token.Token != "" {
				gatewayToken = token.Token
			}
		}

		gtwClient := rtrClient.ForGateway(gatewayID, func() string { return gatewayToken })
		defer gtwClient.Close()

		var downlink <-chan *router.DownlinkMessage
		var errChan <-chan error
		if withDownlink {
			downlink, errChan, err = gtwClient.Subscribe()
			if err != nil {
				ctx.WithError(err).Fatal("Could not start downlink stream")
			}
		}

		m := &util.Message{}
		m.SetDevice(devAddr, nwkSKey, appSKey)
		m.SetMessage(confirmed, fCnt, payload)
		bytes := m.Bytes()

		err = gtwClient.SendUplink(&router.UplinkMessage{
			Payload:          bytes,
			GatewayMetadata:  util.GetGatewayMetadata("ttnctl", 868100000),
			ProtocolMetadata: util.GetProtocolMetadata("SF7BW125"),
		})
		if err != nil {
			ctx.WithError(err).Fatal("Could not send uplink to Router")
		}

		time.Sleep(100 * time.Millisecond)

		ctx.Info("Sent uplink to Router")

		if downlink != nil {
			select {
			case err := <-errChan:
				ctx.WithError(err).Fatal("Error in downlink")
			case downlinkMessage := <-downlink:
				if err := m.Unmarshal(downlinkMessage.Payload); err != nil {
					ctx.WithError(err).Fatal("Could not unmarshal downlink")
				}
				ctx.WithFields(log.Fields{
					"Payload": m.Payload,
					"FCnt":    m.FCnt,
					"FPort":   m.FPort,
				}).Info("Received Downlink")
			case <-time.After(2 * time.Second):
				ctx.Info("Did not receive downlink")
			}
		}
	},
}

func init() {
	RootCmd.AddCommand(uplinkCmd)
	uplinkCmd.Flags().Bool("downlink", false, "Also start downlink (unstable)")
	uplinkCmd.Flags().Bool("confirmed", false, "Use confirmed uplink (this also sets --downlink)")

	uplinkCmd.Flags().String("gateway-id", "", "The ID of the gateway that you are faking (you can only fake gateways that you own)")
	viper.BindPFlag("gateway-id", uplinkCmd.Flags().Lookup("gateway-id"))
}
