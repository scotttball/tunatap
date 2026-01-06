package discovery

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/oracle/oci-go-sdk/v65/identity"
	"github.com/rs/zerolog/log"
	"github.com/scotttball/tunatap/internal/client"
)

// CompartmentNode represents a node in the compartment tree.
type CompartmentNode struct {
	ID       string
	Name     string
	Path     string // Full path like "root/prod/kubernetes"
	ParentID string
	Children []*CompartmentNode
}

// CompartmentTree manages compartment hierarchy for discovery.
type CompartmentTree struct {
	root     *CompartmentNode
	flatList []*CompartmentNode
	mu       sync.RWMutex
}

// BuildCompartmentTree builds a tree of all compartments in the tenancy.
func BuildCompartmentTree(ctx context.Context, ociClient client.OCIClientInterface, tenancyID string) (*CompartmentTree, error) {
	tree := &CompartmentTree{
		root: &CompartmentNode{
			ID:       tenancyID,
			Name:     "root",
			Path:     "root",
			Children: make([]*CompartmentNode, 0),
		},
		flatList: make([]*CompartmentNode, 0),
	}

	// Add root to flat list
	tree.flatList = append(tree.flatList, tree.root)

	// Build tree recursively
	if err := buildTreeRecursive(ctx, ociClient, tree.root, tree); err != nil {
		return nil, fmt.Errorf("failed to build compartment tree: %w", err)
	}

	log.Debug().Msgf("Built compartment tree with %d compartments", len(tree.flatList))
	return tree, nil
}

// buildTreeRecursive recursively builds the compartment tree.
func buildTreeRecursive(ctx context.Context, ociClient client.OCIClientInterface, parent *CompartmentNode, tree *CompartmentTree) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	compartments, err := ociClient.ListCompartments(ctx, parent.ID)
	if err != nil {
		// Log but don't fail - user may not have access to all compartments
		log.Debug().Err(err).Msgf("Failed to list compartments in %s", parent.ID)
		return nil
	}

	for _, c := range compartments {
		if c.Id == nil || c.Name == nil {
			continue
		}

		child := &CompartmentNode{
			ID:       *c.Id,
			Name:     *c.Name,
			Path:     parent.Path + "/" + *c.Name,
			ParentID: parent.ID,
			Children: make([]*CompartmentNode, 0),
		}

		parent.Children = append(parent.Children, child)

		tree.mu.Lock()
		tree.flatList = append(tree.flatList, child)
		tree.mu.Unlock()

		// Recurse into child compartment
		if err := buildTreeRecursive(ctx, ociClient, child, tree); err != nil {
			return err
		}
	}

	return nil
}

// GetRoot returns the root node (tenancy).
func (t *CompartmentTree) GetRoot() *CompartmentNode {
	return t.root
}

// GetFlatList returns all compartments as a flat list.
func (t *CompartmentTree) GetFlatList() []*CompartmentNode {
	t.mu.RLock()
	defer t.mu.RUnlock()

	result := make([]*CompartmentNode, len(t.flatList))
	copy(result, t.flatList)
	return result
}

// FindByPath finds a compartment by its path (e.g., "root/prod/kubernetes").
func (t *CompartmentTree) FindByPath(path string) *CompartmentNode {
	t.mu.RLock()
	defer t.mu.RUnlock()

	for _, node := range t.flatList {
		if node.Path == path {
			return node
		}
	}
	return nil
}

// FindByID finds a compartment by its OCID.
func (t *CompartmentTree) FindByID(id string) *CompartmentNode {
	t.mu.RLock()
	defer t.mu.RUnlock()

	for _, node := range t.flatList {
		if node.ID == id {
			return node
		}
	}
	return nil
}

// FindByName finds all compartments with a given name.
func (t *CompartmentTree) FindByName(name string) []*CompartmentNode {
	t.mu.RLock()
	defer t.mu.RUnlock()

	var matches []*CompartmentNode
	for _, node := range t.flatList {
		if strings.EqualFold(node.Name, name) {
			matches = append(matches, node)
		}
	}
	return matches
}

// Size returns the total number of compartments (including root).
func (t *CompartmentTree) Size() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.flatList)
}

// ForEach iterates over all compartments and calls the callback function.
// If the callback returns an error, iteration stops and the error is returned.
func (t *CompartmentTree) ForEach(fn func(node *CompartmentNode) error) error {
	t.mu.RLock()
	defer t.mu.RUnlock()

	for _, node := range t.flatList {
		if err := fn(node); err != nil {
			return err
		}
	}
	return nil
}

// ForEachParallel iterates over all compartments in parallel.
// maxConcurrency limits concurrent executions (0 = unlimited).
func (t *CompartmentTree) ForEachParallel(ctx context.Context, maxConcurrency int, fn func(ctx context.Context, node *CompartmentNode) error) error {
	t.mu.RLock()
	nodes := make([]*CompartmentNode, len(t.flatList))
	copy(nodes, t.flatList)
	t.mu.RUnlock()

	if maxConcurrency <= 0 {
		maxConcurrency = 10 // Default concurrency
	}

	sem := make(chan struct{}, maxConcurrency)
	errChan := make(chan error, len(nodes))
	var wg sync.WaitGroup

	for _, node := range nodes {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case sem <- struct{}{}:
		}

		wg.Add(1)
		go func(n *CompartmentNode) {
			defer wg.Done()
			defer func() { <-sem }()

			if err := fn(ctx, n); err != nil {
				select {
				case errChan <- err:
				default:
				}
			}
		}(node)
	}

	wg.Wait()
	close(errChan)

	// Return first error if any
	for err := range errChan {
		if err != nil {
			return err
		}
	}

	return nil
}

// GetCompartmentPath builds a readable path from compartment to root.
func GetCompartmentPath(compartments []identity.Compartment, targetID string) string {
	// Build map of ID -> compartment
	byID := make(map[string]identity.Compartment)
	for _, c := range compartments {
		if c.Id != nil {
			byID[*c.Id] = c
		}
	}

	// Walk up the tree
	var path []string
	currentID := targetID

	for {
		c, ok := byID[currentID]
		if !ok {
			break
		}
		if c.Name != nil {
			path = append([]string{*c.Name}, path...)
		}
		if c.CompartmentId == nil {
			break
		}
		currentID = *c.CompartmentId
	}

	if len(path) == 0 {
		return "root"
	}
	return "root/" + strings.Join(path, "/")
}
