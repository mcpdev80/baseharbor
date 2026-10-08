package main

import (
 "github.com/mcpdev80/baseharbor/internal/application"
 "github.com/mcpdev80/baseharbor/internal/externalprovider"
)

const machineCollectionContractVersion = "v1"

type providerListMachineResult struct {
 ContractVersion string `json:"contract_version"`
 Providers []externalprovider.Registration `json:"providers"`
}

func normalizedProviderList(items []externalprovider.Registration) providerListMachineResult {
 if items == nil { items=[]externalprovider.Registration{} }
 return providerListMachineResult{ContractVersion:machineCollectionContractVersion,Providers:items}
}

type connectivityListMachineResult struct {
 ContractVersion string `json:"contract_version"`
 Rules []application.ConnectivityRule `json:"rules"`
}

func normalizedConnectivityList(items []application.ConnectivityRule) connectivityListMachineResult {
 if items==nil {items=[]application.ConnectivityRule{}}
 return connectivityListMachineResult{ContractVersion:machineCollectionContractVersion,Rules:items}
}
