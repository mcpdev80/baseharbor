package runtime

import runtimecontract "github.com/mcpdev80/baseharbor/internal/runtime/contract"

type LogCollectionMode = runtimecontract.LogCollectionMode

const (
	LogCollectionSyslog   = runtimecontract.LogCollectionSyslog
	LogCollectionJournald = runtimecontract.LogCollectionJournald
)

type LogSourceAdapter = runtimecontract.LogSourceAdapter
type RuntimeProvider = runtimecontract.RuntimeProvider

type ProjectResource = runtimecontract.ProjectResource
type RuntimeContainer = runtimecontract.RuntimeContainer
type ImageIdentity = runtimecontract.ImageIdentity
type PublishedPort = runtimecontract.PublishedPort
type ServiceState = runtimecontract.ServiceState

var _ RuntimeProvider = DockerProvider{}
var _ RuntimeProvider = PodmanProvider{}
var _ LogSourceAdapter = DockerProvider{}
var _ LogSourceAdapter = PodmanProvider{}
