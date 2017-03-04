package cmd

import "github.com/DanielKrawisz/bmagent/cmd/rpc"

func buildListAddressesCommand(r *rpc.ListAddressesRequest) (Command, error) {
	// TODO be sure to check that the version is always 4.
	return nil, &ErrUnimplemented{"listaddresses"}
}
