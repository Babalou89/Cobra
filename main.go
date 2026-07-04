// cage — deterministic enforcement cage with the agent worker folded in.
// One binary. Model external over HTTP. No LLM in the verification path.
package main

import "cobra/cmd"

func main() {
	cmd.Execute()
}
