// Command agent is the node agent. It reports a machine's hardware and
// utilisation to the server. Collection lands in the next milestone.
package main

import (
	"fmt"

	"onsite-ai/internal/version"
)

func main() {
	fmt.Printf("onsite agent %s\n", version.Version)
}
