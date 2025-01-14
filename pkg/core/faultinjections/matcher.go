package faultinjections

import (
	"context"

	"github.com/pkg/errors"

	manager_dataplane "github.com/kumahq/kuma/pkg/core/managers/apis/dataplane"
	"github.com/kumahq/kuma/pkg/core/policy"
	core_mesh "github.com/kumahq/kuma/pkg/core/resources/apis/mesh"
	"github.com/kumahq/kuma/pkg/core/resources/manager"
	"github.com/kumahq/kuma/pkg/core/resources/store"
	core_xds "github.com/kumahq/kuma/pkg/core/xds"
)

type FaultInjectionMatcher struct {
	ResourceManager manager.ReadOnlyResourceManager
}

func (f *FaultInjectionMatcher) Match(ctx context.Context, dataplane *core_mesh.DataplaneResource, mesh *core_mesh.MeshResource) (core_xds.FaultInjectionMap, error) {
	faultInjections := &core_mesh.FaultInjectionResourceList{}
	if err := f.ResourceManager.List(ctx, faultInjections, store.ListByMesh(dataplane.GetMeta().GetMesh())); err != nil {
		return nil, errors.Wrap(err, "could not retrieve fault injections")
	}
	return BuildFaultInjectionMap(dataplane, mesh, faultInjections.Items)
}

func BuildFaultInjectionMap(dataplane *core_mesh.DataplaneResource, mesh *core_mesh.MeshResource, faultInjections []*core_mesh.FaultInjectionResource) (core_xds.FaultInjectionMap, error) {
	policies := make([]policy.ConnectionPolicy, len(faultInjections))
	for i, faultInjection := range faultInjections {
		policies[i] = faultInjection
	}

	additionalInbounds, err := manager_dataplane.AdditionalInbounds(dataplane, mesh)
	if err != nil {
		return nil, errors.Wrap(err, "could not fetch additional inbounds")
	}
	inbounds := append(dataplane.Spec.GetNetworking().GetInbound(), additionalInbounds...)
	policyMap := policy.SelectInboundConnectionMatchingPolicies(dataplane, inbounds, policies)

	result := core_xds.FaultInjectionMap{}
	for inbound, connectionPolicies := range policyMap {
		for _, connectionPolicy := range connectionPolicies {
			result[inbound] = append(result[inbound], connectionPolicy.(*core_mesh.FaultInjectionResource).Spec)
		}
	}
	return result, nil
}
