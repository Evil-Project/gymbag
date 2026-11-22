package setup

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"net"
	"slices"
)

const (
	minRandomPort = 20000
	maxRandomPort = 59999
)

// PickRandomFreePort selects a random unused TCP port in [20000, 59999] on 127.0.0.1.
func PickRandomFreePort(exclude ...int) (int, error) {
	span := big.NewInt(maxRandomPort - minRandomPort + 1)
	for attempt := 0; attempt < 64; attempt++ {
		n, err := rand.Int(rand.Reader, span)
		if err != nil {
			return 0, err
		}
		port := minRandomPort + int(n.Int64())
		if slices.Contains(exclude, port) {
			continue
		}
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err != nil {
			continue
		}
		_ = ln.Close()
		return port, nil
	}
	return 0, fmt.Errorf("unable to find a free random TCP port in [%d, %d]", minRandomPort, maxRandomPort)
}
