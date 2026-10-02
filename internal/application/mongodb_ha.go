package application

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

func mongodbMemberCount(m Manifest, instance string) int {
	return managedHAMemberCount(m, "document_database", 3)
}

func mongodbMemberServiceName(instance string, ordinal int) string {
	base := runtimeServiceName("mongodb", instance)
	if ordinal <= 0 {
		return base
	}
	return fmt.Sprintf("%s-%d", base, ordinal+1)
}

func mongodbMemberVolumeName(instance string, ordinal int) string {
	return mongodbMemberServiceName(instance, ordinal) + "-data"
}

func mongodbMemberAccessService(instance string, ordinal int) string {
	base := mongodbAccessService(instance)
	if ordinal <= 0 {
		return base
	}
	return fmt.Sprintf("%s-%d", base, ordinal+1)
}

func mongodbMemberHostPortKey(instance string, ordinal int) string {
	if ordinal <= 0 {
		return mongodbRuntimeKey(instance, "HOST_PORT")
	}
	return mongodbRuntimeKey(instance, fmt.Sprintf("HOST_PORT_%d", ordinal+1))
}

func mongodbReplicaSetKey(instance string) string {
	return mongodbRuntimeKey(instance, "REPLICA_SET")
}

func mongodbReplicaKeyKey(instance string) string {
	return mongodbRuntimeKey(instance, "REPLICA_KEY")
}

func mongodbReplicaSetName(instance string) string {
	token := strings.ToLower(strings.ReplaceAll(instance, "-", "_"))
	if token == "" || token == defaultServiceInstance {
		return "baseharbor"
	}
	return "baseharbor_" + token
}

func mongodbSeedURI(hosts []string, database, username, password, replicaSet string) string {
	q := url.Values{}
	q.Set("authSource", database)
	q.Set("tls", "true")
	if strings.TrimSpace(replicaSet) != "" {
		q.Set("replicaSet", replicaSet)
	}
	escapedHosts := make([]string, 0, len(hosts))
	for _, host := range hosts {
		host = strings.TrimSpace(host)
		if host == "" {
			continue
		}
		if _, _, err := net.SplitHostPort(host); err != nil {
			host = net.JoinHostPort(host, "27017")
		}
		escapedHosts = append(escapedHosts, host)
	}
	return "mongodb://" + url.UserPassword(username, password).String() + "@" + strings.Join(escapedHosts, ",") + "/" + url.PathEscape(database) + "?" + q.Encode()
}

func mongodbContainerSeedHosts(m Manifest, instance string) []string {
	count := mongodbMemberCount(m, instance)
	hosts := make([]string, 0, count)
	for ordinal := 0; ordinal < count; ordinal++ {
		hosts = append(hosts, net.JoinHostPort(mongodbMemberServiceName(instance, ordinal), "27017"))
	}
	return hosts
}

func mongodbHostSeedHosts(m Manifest, values map[string]string, instance string) ([]string, error) {
	count := mongodbMemberCount(m, instance)
	hosts := make([]string, 0, count)
	for ordinal := 0; ordinal < count; ordinal++ {
		port, err := requireRuntimeValue(values, mongodbMemberHostPortKey(instance, ordinal))
		if err != nil {
			return nil, err
		}
		hosts = append(hosts, net.JoinHostPort(loopbackHost, port))
	}
	return hosts, nil
}

