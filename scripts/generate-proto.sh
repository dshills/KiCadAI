#!/usr/bin/env bash
set -euo pipefail

project_root="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
proto_root="$project_root/third_party/kicad/api/proto"
out_dir="$project_root/internal/kiapi/gen"
module_path="kicadai/internal/kiapi/gen"

if ! command -v protoc >/dev/null 2>&1; then
	echo "protoc is required but was not found in PATH." >&2
	exit 1
fi

if ! command -v protoc-gen-go >/dev/null 2>&1; then
	echo "protoc-gen-go is required but was not found in PATH." >&2
	echo "Install it with: GOBIN=$project_root/bin go install google.golang.org/protobuf/cmd/protoc-gen-go@latest" >&2
	exit 1
fi

rm -rf "$out_dir"
mkdir -p "$out_dir"

map_options=(
	"Mboard/board.proto=$module_path/board;board"
	"Mboard/board_commands.proto=$module_path/board/commands;boardcommands"
	"Mboard/board_jobs.proto=$module_path/board/jobs;boardjobs"
	"Mboard/board_types.proto=$module_path/board/types;boardtypes"
	"Mcommon/commands/base_commands.proto=$module_path/common/commands;commoncommands"
	"Mcommon/commands/editor_commands.proto=$module_path/common/commands;commoncommands"
	"Mcommon/commands/project_commands.proto=$module_path/common/commands;commoncommands"
	"Mcommon/envelope.proto=$module_path/common;common"
	"Mcommon/types/base_types.proto=$module_path/common/types;commontypes"
	"Mcommon/types/enums.proto=$module_path/common/types;commontypes"
	"Mcommon/types/jobs.proto=$module_path/common/types;commontypes"
	"Mcommon/types/project_settings.proto=$module_path/common/project;commonproject"
	"Mcommon/types/wizards.proto=$module_path/common/types;commontypes"
	"Mschematic/schematic_commands.proto=$module_path/schematic/types;schematictypes"
	"Mschematic/schematic_jobs.proto=$module_path/schematic/jobs;schematicjobs"
	"Mschematic/schematic_types.proto=$module_path/schematic/types;schematictypes"
)

proto_files=()
while IFS= read -r proto_file; do
	proto_files+=("${proto_file#"$proto_root/"}")
done < <(find "$proto_root" -name '*.proto' | sort)

(
	cd "$proto_root"
	protoc \
		--go_out="$out_dir" \
		--go_opt=module="$module_path" \
		"${map_options[@]/#/--go_opt=}" \
		"${proto_files[@]}"
)

cat > "$out_dir/README.md" <<'EOF'
# Generated KiCad Protobuf Bindings

This directory is generated from `third_party/kicad/api/proto`.

Regenerate with:

```sh
make proto
```

Do not edit generated `.pb.go` files by hand.
EOF
