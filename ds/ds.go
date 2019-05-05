// ds is a package for using Google Cloud Datastore.
package ds

import (
	"context"
	"fmt"

	"cloud.google.com/go/datastore"
)

type Kind string

type DS struct {
	// Client is the Cloud Datastore client.
	Client *datastore.Client

	// Namespace is the datastore namespace that data will be stored in.
	Namespace string
}

// InitWithOpt the Cloud Datastore Client (DS).
//
// project - The project name, i.e. "google.com:skia-buildbots".
// ns      - The datastore namespace to store data into.
// opt     - Options to pass to the client.
func New(ctx context.Context, project string, ns string) (*DS, error) {
	if ns == "" {
		return nil, fmt.Errorf("Datastore namespace cannot be empty.")
	}

	client, err := datastore.NewClient(ctx, project)
	if err != nil {
		return nil, fmt.Errorf("Failed to initialize Cloud Datastore: %s", err)
	}
	return &DS{
		Namespace: ns,
		Client:    client,
	}, nil
}

// Creates a new indeterminate key of the given kind.
func (ds *DS) NewKey(kind Kind) *datastore.Key {
	return &datastore.Key{
		Kind:      string(kind),
		Namespace: ds.Namespace,
	}
}

func (ds *DS) NewKeyWithParent(kind Kind, parent *datastore.Key) *datastore.Key {
	ret := ds.NewKey(kind)
	ret.Parent = parent
	return ret
}

// Creates a new query of the given kind with the right namespace.
func (ds *DS) NewQuery(kind Kind) *datastore.Query {
	return datastore.NewQuery(string(kind)).Namespace(ds.Namespace)
}
