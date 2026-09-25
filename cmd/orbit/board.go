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
)

// runBoard is `orbit board`: read a project's board, or act. A read is a
// read — it loads config in read mode (ORBIT_RPC only, no vault creator,
// no key), like project list and agent list. An act is a write: the agent
// key and vault creator are required, and the indexer is optional — with
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
	mint := rest[0]
	project := board.Project{Mint: mint}
	if len(rest) == 1 || rest[1] == "read" {
		return boardRead(ctx, project)
	}
	return boardAct(ctx, project, rest[1], rest[2:])
}

// boardRead is the board's read mode: RPC only, no vault creator, no key.
func boardRead(ctx context.Context, project board.Project) int {
	cfg, err := client.LoadBoardReadConfig(os.Getenv)
	if err != nil {
		die("board: %v", err)
	}
	tc, err := client.NewRead(cfg)
	if err != nil {
		die("board: %v", err)
	}
	db, err := board.Open(board.StorePath(rigHome()))
	if err != nil {
		die("board: store: %v", err)
	}
	defer db.DB.Close()
	st := &board.Store{Client: tc, DB: db}
	reply, err := st.Board(ctx, project)
	if err != nil {
		die("board: read %s: %v", project.Mint, err)
	}
	fmt.Print(reply)
	return 0
}

// boardAct is the board's write mode: the agent key and vault creator are
// required, and any wallet may act — ownership and stake are the fold's.
func boardAct(ctx context.Context, project board.Project, verb string, rest []string) int {
	cfg, err := client.LoadBoardConfig(os.Getenv)
	if err != nil {
		die("board: %v", err)
	}
	tc, err := client.New(cfg)
	if err != nil {
		die("board: %v", err)
	}
	db, err := board.Open(board.StorePath(rigHome()))
	if err != nil {
		die("board: store: %v", err)
	}
	defer db.DB.Close()
	st := &board.Store{Client: tc, DB: db}
	var reply string
	switch verb {
	case "task":
		text := strings.Join(rest, " ")
		if text == "" {
			die("board: task needs text")
		}
		next, err := st.NextID(ctx, project)
		if err != nil {
			die("board: task id: %v", err)
		}
		reply, err = st.Act(ctx, project, board.Shape{Verb: "task", ID: next, Text: text})
		if err != nil {
			die("board: %v", err)
		}
	case "brief", "note", "reject":
		if len(rest) < 2 {
			die("board: %s needs a task id and text", verb)
		}
		id, err := strconv.Atoi(rest[0])
		if err != nil {
			die("board: bad task id %q", rest[0])
		}
		text := strings.Join(rest[1:], " ")
		reply, err = st.Act(ctx, project, board.Shape{Verb: verb, ID: id, Text: text})
		if err != nil {
			die("board: %v", err)
		}
	case "claim", "complete", "accept":
		if len(rest) < 1 {
			die("board: %s needs a task id", verb)
		}
		id, err := strconv.Atoi(rest[0])
		if err != nil {
			die("board: bad task id %q", rest[0])
		}
		reply, err = st.Act(ctx, project, board.Shape{Verb: verb, ID: id})
		if err != nil {
			die("board: %v", err)
		}
	default:
		die("board: unknown action %q (read|task|brief|claim|note|complete|accept|reject)", verb)
	}
	fmt.Println(reply)
	return 0
}
