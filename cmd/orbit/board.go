package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/mrsirg97-rgb/orbit/board"
	"github.com/mrsirg97-rgb/orbit/client"
	"github.com/mrsirg97-rgb/orbit/identity"
)

// runBoard is `orbit board`: read a project's board, or act. The role comes
// from the identity row (--role overrides); the indexer is optional — with
// ORBIT_INDEXER unset the board reads the chain directly.
//
//	orbit board <mint> [read]
//	orbit board <mint> task <text>
//	orbit board <mint> brief <id> <text>
//	orbit board <mint> claim <id>
//	orbit board <mint> note <id> <text>
//	orbit board <mint> complete <id>
//	orbit board <mint> accept <id>
//	orbit board <mint> reject <id> <reason>
func runBoard(args []string) int {
	fs := flag.NewFlagSet("board", flag.ContinueOnError)
	roleFlag := fs.String("role", "", "override the identity row's role (architect | worker | reviewer)")
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	rest := fs.Args()
	if len(rest) < 1 {
		fmt.Fprintln(os.Stderr, "orbit: usage: board <mint> [read|task|brief|claim|note|complete|accept|reject ...]")
		return 2
	}
	ctx := context.Background()
	cfg, err := client.LoadBoardConfig(os.Getenv)
	if err != nil {
		die("board: %v", err)
	}
	tc, err := client.New(cfg)
	if err != nil {
		die("board: %v", err)
	}
	role := *roleFlag
	if role == "" {
		role, _ = workerRole()
	}
	if _, err := identity.ParseRole(role); err != nil {
		die("board: %v", err)
	}
	db, err := board.Open(board.StorePath(rigHome()))
	if err != nil {
		die("board: store: %v", err)
	}
	defer db.DB.Close()
	st := &board.Store{Client: tc, DB: db, Role: role}
	mint := rest[0]
	project := board.Project{Mint: mint}
	if len(rest) == 1 || rest[1] == "read" {
		reply, err := st.Board(ctx, project)
		if err != nil {
			die("board: read %s: %v", mint, err)
		}
		fmt.Print(reply)
		return 0
	}
	verb := rest[1]
	switch verb {
	case "task":
		text := strings.Join(rest[2:], " ")
		if text == "" {
			die("board: task needs text")
		}
		next, err := st.NextID(ctx, project)
		if err != nil {
			die("board: task id: %v", err)
		}
		reply, err := st.Act(ctx, project, board.Shape{Role: role, Verb: "task", ID: next, Text: text})
		if err != nil {
			die("board: %v", err)
		}
		fmt.Println(reply)
	case "brief", "note", "reject":
		if len(rest) < 3 {
			die("board: %s needs a task id and text", verb)
		}
		id, err := strconv.Atoi(rest[2])
		if err != nil {
			die("board: bad task id %q", rest[2])
		}
		text := strings.Join(rest[3:], " ")
		reply, err := st.Act(ctx, project, board.Shape{Role: role, Verb: verb, ID: id, Text: text})
		if err != nil {
			die("board: %v", err)
		}
		fmt.Println(reply)
	case "claim", "complete", "accept":
		if len(rest) < 3 {
			die("board: %s needs a task id", verb)
		}
		id, err := strconv.Atoi(rest[2])
		if err != nil {
			die("board: bad task id %q", rest[2])
		}
		reply, err := st.Act(ctx, project, board.Shape{Role: role, Verb: verb, ID: id})
		if err != nil {
			die("board: %v", err)
		}
		fmt.Println(reply)
	default:
		die("board: unknown action %q (read|task|brief|claim|note|complete|accept|reject)", verb)
	}
	return 0
}

func workerRole() (string, error) {
	ctx := context.Background()
	idb := identityStore()
	defer idb.DB.Close()
	id := os.Getenv("ORBIT_AGENT_ID")
	if id != "" {
		row, err := identity.GetByID(ctx, idb, id)
		if err != nil {
			return "", err
		}
		return row.Role, nil
	}
	row, err := identity.Get(ctx, idb)
	if err != nil {
		return "", err
	}
	return row.Role, nil
}
