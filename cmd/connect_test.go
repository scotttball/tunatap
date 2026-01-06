package cmd

import (
	"testing"

	"github.com/scotttball/tunatap/internal/config"
)

func TestSelectClusterByName(t *testing.T) {
	cfg := &config.Config{
		Clusters: []*config.Cluster{
			{ClusterName: "cluster-1", Region: "us-ashburn-1"},
			{ClusterName: "cluster-2", Region: "eu-frankfurt-1"},
		},
	}

	// Test finding by name
	cluster, err := selectCluster(cfg, "cluster-1")
	if err != nil {
		t.Fatalf("selectCluster() error = %v", err)
	}

	if cluster.ClusterName != "cluster-1" {
		t.Errorf("ClusterName = %q, want %q", cluster.ClusterName, "cluster-1")
	}
}

func TestSelectClusterNotFound(t *testing.T) {
	cfg := &config.Config{
		Clusters: []*config.Cluster{
			{ClusterName: "cluster-1", Region: "us-ashburn-1"},
		},
	}

	_, err := selectCluster(cfg, "nonexistent")
	if err == nil {
		t.Error("selectCluster() should error for non-existent cluster")
	}
}

func TestSelectClusterEmpty(t *testing.T) {
	cfg := &config.Config{
		Clusters: []*config.Cluster{},
	}

	_, err := selectCluster(cfg, "")
	if err == nil {
		t.Error("selectCluster() should error for empty cluster list")
	}
}

func TestSelectClusterSingleCluster(t *testing.T) {
	cfg := &config.Config{
		Clusters: []*config.Cluster{
			{ClusterName: "only-cluster", Region: "us-ashburn-1"},
		},
	}

	// With empty name and single cluster, should return that cluster
	cluster, err := selectCluster(cfg, "")
	if err != nil {
		t.Fatalf("selectCluster() error = %v", err)
	}

	if cluster.ClusterName != "only-cluster" {
		t.Errorf("ClusterName = %q, want %q", cluster.ClusterName, "only-cluster")
	}
}

func TestConnectCommandExists(t *testing.T) {
	if connectCmd == nil {
		t.Fatal("connectCmd is nil")
	}

	if connectCmd.Use != "connect [cluster]" {
		t.Errorf("connectCmd.Use = %q, want %q", connectCmd.Use, "connect [cluster]")
	}
}

func TestConnectCommandFlags(t *testing.T) {
	// Test that expected flags exist
	flag := connectCmd.Flags().Lookup("cluster")
	if flag == nil {
		t.Error("--cluster flag not found")
	}

	flag = connectCmd.Flags().Lookup("port")
	if flag == nil {
		t.Error("--port flag not found")
	}

	flag = connectCmd.Flags().Lookup("bastion")
	if flag == nil {
		t.Error("--bastion flag not found")
	}

	flag = connectCmd.Flags().Lookup("endpoint")
	if flag == nil {
		t.Error("--endpoint flag not found")
	}

	flag = connectCmd.Flags().Lookup("no-bastion")
	if flag == nil {
		t.Error("--no-bastion flag not found")
	}
}
