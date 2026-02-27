package openbaocluster

import appopenbaocluster "github.com/dc-tec/openbao-operator/internal/app/openbaocluster"

// clusterState aliases the app-layer status model so existing status helpers keep
// their local signatures while state gathering lives in the app layer.
type clusterState = appopenbaocluster.StatusState
