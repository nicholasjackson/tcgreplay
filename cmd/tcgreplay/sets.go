package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"maps"
	"os"
	"os/signal"
	"slices"
	"strings"
	"syscall"

	"github.com/nicholasjackson/tcgreplay/internal/cardset"
	"github.com/nicholasjackson/tcgreplay/internal/spaces"
)

const setsUsage = `usage:
  tcgreplay sets import <set-id|code> [-dir cardsets] [-no-malie]
  tcgreplay sets import -all [-dir cardsets] [-no-malie]
  tcgreplay sets check [<set>...] [-dir cardsets]
  tcgreplay sets publish [-dir cardsets] [-to <directory>] [-dry-run]
`

// runSets runs the sets subcommand, which builds card sets into the
// project ahead of time (see internal/cardset), and returns the exit code.
func runSets(args []string) int {
	return runSetsTo(args, os.Stdout, os.Stderr)
}

func runSetsTo(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, setsUsage)
		return 2
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	switch args[0] {
	case "import":
		return setsImport(ctx, args[1:], stdout, stderr)
	case "check":
		return setsCheck(args[1:], stdout, stderr)
	case "publish":
		return setsPublish(ctx, args[1:], os.Getenv, stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown sets command %q\n", args[0])
		fmt.Fprint(stderr, setsUsage)
		return 2
	}
}

// parseInterspersed parses fs from args allowing flags after positional
// words ("import me06 -dir x"), returning the positional words.
func parseInterspersed(fs *flag.FlagSet, args []string) ([]string, error) {
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		if fs.NArg() == 0 {
			return pos, nil
		}
		pos = append(pos, fs.Arg(0))
		args = fs.Args()[1:]
	}
}

func setsImport(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("tcgreplay sets import", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dir := fs.String("dir", cardset.DefaultDir, "card-set directory")
	noMalie := fs.Bool("no-malie", false, "build from TCGdex alone, without the malie.io TCG Live export")
	tcgdexBase := fs.String("tcgdex-base", "", "TCGdex API base URL (tests)")
	malieBase := fs.String("malie-base", "", "malie.io export base URL (tests)")
	all := fs.Bool("all", false, "import every English set TCGdex lists (Pokémon TCG Pocket aside)")
	pos, err := parseInterspersed(fs, args)
	if err != nil {
		return 2
	}
	if *all == (len(pos) == 1) || len(pos) > 1 {
		fmt.Fprintln(stderr, "sets import needs exactly one set id or decklist code, or -all")
		fmt.Fprint(stderr, setsUsage)
		return 2
	}
	cardset.UserAgent = "tcgreplay-sets/" + version
	opts := cardset.ImportOptions{
		Dir:     *dir,
		NoMalie: *noMalie,
		TCGdex:  &cardset.TCGdex{Base: *tcgdexBase},
		Malie:   &cardset.Malie{Base: *malieBase},
		Log:     stderr,
	}
	if *all {
		return setsImportAll(ctx, opts, stdout, stderr)
	}
	opts.Set = pos[0]
	sum, err := cardset.Import(ctx, opts)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintln(stdout, sum)
	return 0
}

// setsCheck runs the offline checks over built sets and prints every open
// item; it exits 1 when there is any.
func setsCheck(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("tcgreplay sets check", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dir := fs.String("dir", cardset.DefaultDir, "card-set directory")
	sets, err := parseInterspersed(fs, args)
	if err != nil {
		return 2
	}
	items, err := cardset.Check(*dir, sets...)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	for _, it := range items {
		fmt.Fprintln(stdout, it)
	}
	if len(items) > 0 {
		fmt.Fprintf(stderr, "%d open items\n", len(items))
		return 1
	}
	fmt.Fprintln(stdout, "all card sets check out")
	return 0
}

// setsImportAll imports every set, printing one summary line per set, then
// the sets that failed and the malie sets no TCGdex set matches. It exits
// 1 if any set failed.
func setsImportAll(ctx context.Context, opts cardset.ImportOptions, stdout, stderr io.Writer) int {
	res, err := cardset.ImportAll(ctx, opts)
	for _, s := range res.Summaries {
		fmt.Fprintln(stdout, s)
	}
	open := 0
	for _, s := range res.Summaries {
		open += s.Open
	}
	fmt.Fprintf(stdout, "%d sets built, %d open items, %d failed, %d Pokémon TCG Pocket sets skipped\n", len(res.Summaries), open, len(res.Failed), len(res.Skipped))
	for _, id := range slices.Sorted(maps.Keys(res.Failed)) {
		fmt.Fprintf(stdout, "failed: %s: %v\n", id, res.Failed[id])
	}
	for _, u := range res.Unmatched {
		fmt.Fprintf(stdout, "malie set with no TCGdex set: %s (%s, %d entries)\n", u.Key, u.Abbr, u.Entries)
	}
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if len(res.Failed) > 0 {
		return 1
	}
	return 0
}

// envCDNToken is the DigitalOcean API token the publish purges the CDN
// with; the hosted playmat reads the same setting.
const envCDNToken = "TCGREPLAY_SPACES_CDN_TOKEN"

// setsPublish publishes the built sets: to the bucket the TCGREPLAY_SPACES_*
// settings name (purging the manifest from the CDN when
// TCGREPLAY_SPACES_CDN_TOKEN is set), or to a local directory with -to. It
// prints what it uploaded and what was unchanged, and exits 1 when the
// publish fails (open check items, a published set missing from the
// directory, an upload or purge failure) and 2 when the settings are
// missing or partial.
func setsPublish(ctx context.Context, args []string, getenv func(string) string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("tcgreplay sets publish", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dir := fs.String("dir", cardset.DefaultDir, "card-set directory")
	to := fs.String("to", "", "publish to this local directory instead of the bucket")
	dryRun := fs.Bool("dry-run", false, "report what would be uploaded, writing nothing")
	cdnAPI := fs.String("cdn-api", "", "DigitalOcean API base URL (tests)")
	pos, err := parseInterspersed(fs, args)
	if err != nil {
		return 2
	}
	if len(pos) > 0 {
		fmt.Fprintf(stderr, "sets publish takes no set names (it publishes every built set): %q\n", pos)
		fmt.Fprint(stderr, setsUsage)
		return 2
	}
	opts := cardset.PublishOptions{
		Dir:    *dir,
		DryRun: *dryRun,
		Logf:   func(format string, a ...any) { fmt.Fprintf(stderr, format+"\n", a...) },
	}
	where := *to
	if *to != "" {
		opts.Target = cardset.DirTarget{Dir: *to}
	} else {
		cfg, set, err := spaces.FromEnv(getenv)
		if !set {
			fmt.Fprintln(stderr, "set TCGREPLAY_SPACES_ENDPOINT, _BUCKET, _KEY and _SECRET, or use -to")
			return 2
		}
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 2
		}
		c, err := spaces.New(cfg)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 2
		}
		opts.Target = cardset.SpacesTarget{C: c}
		opts.Prefix = cardset.BucketPrefix
		where = "bucket " + cfg.Bucket
		if token := strings.TrimSpace(getenv(envCDNToken)); token != "" {
			cdn := spaces.NewCDN(cfg, token)
			if *cdnAPI != "" {
				cdn.API = *cdnAPI
			}
			opts.Purge = cdn.Purge
		}
	}
	res, err := cardset.Publish(ctx, opts)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	verb := "uploaded"
	if *dryRun {
		verb = "would upload"
	}
	for _, id := range res.Uploaded {
		fmt.Fprintf(stdout, "%s %s (%d prints)\n", verb, id, res.Manifest.Sets[id].Prints)
	}
	fmt.Fprintf(stdout, "unchanged: %d sets\n", len(res.Unchanged))
	switch {
	case !res.ManifestChanged:
		fmt.Fprintln(stdout, "manifest unchanged")
	case *dryRun:
		fmt.Fprintf(stdout, "manifest would be updated (dry run: nothing written to %s)\n", where)
	default:
		fmt.Fprintf(stdout, "manifest updated in %s\n", where)
	}
	return 0
}
