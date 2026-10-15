package main

import (
	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/devgateway"
	"testing"
)

func TestIdentityRetirementKeepsVerifiedDestinationAndWorkloadRoutes(t *testing.T) {
	resolved := resolvedApplication{Manifest: application.New("demo", "dev", false, false, false)}
	owner := "app/demo/dev"
	routes := []devgateway.Route{
		{Owner: owner, Key: owner + "/identity", Network: "new-provider"},
		{Owner: owner, Key: owner + "/identity-admin", Network: "old-provider"},
		{Owner: owner, Key: owner + "/workload-api", Network: "old-provider"},
		{Owner: "shared/keycloak", Key: "shared/keycloak/login", Network: "old-provider"},
		{Owner: "app/sibling/dev", Key: "app/sibling/dev/identity", Network: "old-provider"},
	}
	groups := retainedIdentityGatewayOwnerRoutes(routes, resolved, "old-provider")
	if len(groups) != 2 || groups[0].Owner != owner || len(groups[0].Routes) != 2 || groups[0].Routes[0].Network != "new-provider" || groups[0].Routes[1].Key != owner+"/workload-api" || groups[1].Owner != "shared/keycloak" || len(groups[1].Routes) != 0 {
		t.Fatal("old identity retirement removed verified destination or workload routes, or retained old login route")
	}
	if len(retainedIdentityGatewayOwnerRoutes(routes, resolved, "unknown-network")) != 0 {
		t.Fatal("unknown realization acquired route removal authority")
	}
}
