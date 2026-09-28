// Command gameserver 是 Go 控制面的组合根。
// 当前为工具链与模块边界骨架；HTTP 服务装配在后续 implementation tasks 中按 ADR-001 §2.1 补齐。
package main

import (
	"fmt"

	"github.com/mj8724/gameserver/internal/version"
)

func main() {
	fmt.Printf("gameserver %s (skeleton; see docs/adr/ADR-001-go-backend.md)\n", version.String())
}
