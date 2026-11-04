package cards

import "context"

// Warm builds the card catalog, the set list and every set's card list,
// through the normal lookup path: from the store where it holds them,
// from the card source otherwise. It is for start-up, so the first lookup
// that needs the catalog does not wait for it, and is not part of DB.
func (c *Client) Warm(ctx context.Context) error {
	_, err := c.ensureCatalog(ctx)
	return err
}
