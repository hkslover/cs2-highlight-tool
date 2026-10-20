//go:build !windows

package steam

import "context"

// newGCSession reports that the Steam Game Coordinator binding is Windows only.
// The rest of the package (share codes, match list decoding, demo download)
// builds and is tested everywhere; only loading the local Steam library and
// talking to the Game Coordinator is Windows specific.
func newGCSession(_ context.Context, _ ListOptions) (gcSession, error) {
	return nil, ErrUnsupportedOS
}
