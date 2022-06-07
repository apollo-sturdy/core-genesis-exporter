package terraswap

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/terra-money/core/app/export/util"
)

// see if pool contains any of LUNA, UST, AUST, BLUNA
func isTargetPool(p *pool) bool {
	isOk := (p.Assets[0].AssetInfo.NativeToken != nil && p.Assets[0].AssetInfo.NativeToken.Denom == util.DenomLUNA) ||
		(p.Assets[0].AssetInfo.NativeToken != nil && p.Assets[0].AssetInfo.NativeToken.Denom == util.DenomUST) ||
		(p.Assets[0].AssetInfo.Token != nil && p.Assets[0].AssetInfo.Token.ContractAddr == AddressAUST) ||
		(p.Assets[0].AssetInfo.Token != nil && p.Assets[0].AssetInfo.Token.ContractAddr == AddressBLUNA) ||
		(p.Assets[0].AssetInfo.Token != nil && p.Assets[0].AssetInfo.Token.ContractAddr == AddressSTLUNA) ||
		(p.Assets[0].AssetInfo.Token != nil && p.Assets[0].AssetInfo.Token.ContractAddr == AddressCLUNA) ||
		(p.Assets[0].AssetInfo.Token != nil && p.Assets[0].AssetInfo.Token.ContractAddr == AddressPLUNA) ||
		(p.Assets[0].AssetInfo.Token != nil && p.Assets[0].AssetInfo.Token.ContractAddr == AddressNLUNA) ||
		(p.Assets[0].AssetInfo.Token != nil && p.Assets[0].AssetInfo.Token.ContractAddr == AddressSTEAK) ||
		(p.Assets[0].AssetInfo.Token != nil && p.Assets[0].AssetInfo.Token.ContractAddr == AddressLUNAX) ||
		(p.Assets[1].AssetInfo.NativeToken != nil && p.Assets[1].AssetInfo.NativeToken.Denom == util.DenomLUNA) ||
		(p.Assets[1].AssetInfo.NativeToken != nil && p.Assets[1].AssetInfo.NativeToken.Denom == util.DenomUST) ||
		(p.Assets[1].AssetInfo.Token != nil && p.Assets[1].AssetInfo.Token.ContractAddr == AddressAUST) ||
		(p.Assets[1].AssetInfo.Token != nil && p.Assets[1].AssetInfo.Token.ContractAddr == AddressBLUNA) ||
		(p.Assets[1].AssetInfo.Token != nil && p.Assets[1].AssetInfo.Token.ContractAddr == AddressSTLUNA) ||
		(p.Assets[1].AssetInfo.Token != nil && p.Assets[1].AssetInfo.Token.ContractAddr == AddressCLUNA) ||
		(p.Assets[1].AssetInfo.Token != nil && p.Assets[1].AssetInfo.Token.ContractAddr == AddressPLUNA) ||
		(p.Assets[1].AssetInfo.Token != nil && p.Assets[1].AssetInfo.Token.ContractAddr == AddressNLUNA) ||
		(p.Assets[1].AssetInfo.Token != nil && p.Assets[1].AssetInfo.Token.ContractAddr == AddressSTEAK) ||
		(p.Assets[1].AssetInfo.Token != nil && p.Assets[1].AssetInfo.Token.ContractAddr == AddressLUNAX)

	return isOk
}

func pickDenomOrContractAddress(asset assetInfo) string {
	if asset.Token != nil {
		return asset.Token.ContractAddr
	}

	if asset.NativeToken != nil {
		return asset.NativeToken.Denom
	}

	panic("unknown denom")
}

func coalesceToBalanceDenom(assetName string) (string, bool) {
	switch assetName {
	case "uusd":
		return util.DenomUST, true
	case "uluna":
		return util.DenomLUNA, true
	case AddressAUST:
		return util.DenomAUST, true
	case AddressBLUNA:
		return util.DenomBLUNA, true
	case AddressSTLUNA:
		return util.DenomSTLUNA, true
	case AddressCLUNA:
		return util.DenomCLUNA, true
	case AddressPLUNA:
		return util.DenomPLUNA, true
	case AddressNLUNA:
		return util.DenomNLUNA, true
	case AddressSTEAK:
		return util.DenomSTEAK, true
	case AddressLUNAX:
		return util.DenomLUNAX, true
	}

	return "", false
}

func getShareInAssets(p pool, lpAmount sdk.Int, totalShare sdk.Int) [2]sdk.Int {
	shareRatio := sdk.ZeroDec()
	if !totalShare.IsZero() {
		shareRatio = sdk.NewDecFromInt(lpAmount).Quo(sdk.NewDecFromInt(totalShare))
	}

	return [2]sdk.Int{
		shareRatio.MulInt(p.Assets[0].Amount).TruncateInt(),
		shareRatio.MulInt(p.Assets[1].Amount).TruncateInt(),
	}
}
