package cmd

import (
	"fmt"
	"github.com/spf13/cobra"
)

func compactCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "compact [path_to_home]",
		Short: "compact data from the application store and block store",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if tendermint {
				blockStoreDB, errDBBlock := openCometBFTDB("blockstore", args[0])
				defer blockStoreDB.Close()

				if errDBBlock != nil {
					return errDBBlock
				}
				if err := compactCometBFTDB(blockStoreDB); err != nil {
					fmt.Println(err.Error())
				}

				stateDB, errDBBState := openCometBFTDB("state", args[0])
				defer stateDB.Close()

				if errDBBState != nil {
					return errDBBState
				}
				if err := compactCometBFTDB(stateDB); err != nil {
					fmt.Println(err.Error())
				}
			}

			if cosmosSdk {
				appDB, errDB := openCosmosDB("application", args[0])
				defer appDB.Close()

				if errDB != nil {
					return errDB
				}
				if err := compactCosmosDB(appDB); err != nil {
					fmt.Println(err.Error())
				}
			}

			if tx_idx {
				txIdxDB, err := openCosmosDB("tx_index", args[0])
				if err != nil {
					return err
				}

				defer func() {
					errClose := txIdxDB.Close()
					if errClose != nil {
						fmt.Println(errClose.Error())
					}
				}()

				if err := compactCosmosDB(txIdxDB); err != nil {
					fmt.Println(err.Error())
				}
			}

			return nil
		},
	}
	return cmd
}
