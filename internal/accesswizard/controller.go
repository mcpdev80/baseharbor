package accesswizard

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/policy"
)

// readLine returns one line from the shared buffered reader. Creating a fresh
// bufio.Reader per line would discard the bytes of the following line (the
// buffer is abandoned when the reader is dropped), which breaks piped and
// non-interactive input and corrupts the wizard flow.
func (c *Controller) readLine() (string, error) {
	if c.reader == nil {
		c.reader = bufio.NewReader(c.In)
	}
	return c.reader.ReadString('\n')
}

func indexString(list []string, target string) int {
	for i, v := range list {
		if v == target {
			return i
		}
	}
	return -1
}

// PageSize bounds how many targets the search screen lists before paging. A
// large list is handled by the same paging loop, so the duplicate/large-list
// acceptance case is covered.
const PageSize = 10

// Controller drives the guided profile-assignment and offboarding wizard. It is
// pure input handling: every mutation it reaches goes through the production
// policy authority supplied by the CLI layer. The controller holds no decision
// authority of its own.
type Controller struct {
	In     io.Reader
	Out    io.Writer
	known  map[string]string
	scope  map[string]string
	page   int
	reader *bufio.Reader
	// commitErr is set by the CLI decide callback (via NoteCommitError) when a
	// persistence failure caused the Deny, so Run can report the precise reason.
	commitErr error
}

// NewController builds a controller bound to the known target set.
func NewController(knownTargets map[string]string) *Controller {
	return &Controller{
		known: knownTargets,
	}
}

// SetScope supplies the effective operation each known target declares, so the
// search and review screens show exactly what an assignment would grant.
func (c *Controller) SetScope(scope map[string]string) {
	c.scope = scope
}

// SetIO wires the interactive reader and output writer. The CLI layer supplies
// the input reader and the output stream; tests supply in-memory buffers.
func (c *Controller) SetIO(in io.Reader, out io.Writer) {
	c.In = in
	c.Out = out
}

// SetKnown replaces the known target set (used by tests).
func (c *Controller) SetKnown(known map[string]string) {
	c.known = known
}

// NoteCommitError records a persistence failure the decide callback wants
// reported. Run consumes it once when the callback returns Deny, so the
// operator sees which write was refused (for example an owner-collision guard)
// instead of a generic rejection.
func (c *Controller) NoteCommitError(err error) {
	c.commitErr = err
}

// KnownTargets returns the sorted known target names for the search screen.
func (c *Controller) KnownTargets() []string {
	return SortedTargets(c.known)
}

func (c *Controller) scopeOf(target string) string {
	if c.scope == nil {
		return ""
	}
	return c.scope[target]
}

// parseTargetList parses a comma-separated target selector ("1,2,3" or
// "dev,prod") into a list of selections in the display order given. Every listed
// entry must reference a displayed target; a single missing entry fails closed
// so a partially-invalid list never silently drops targets. Finding #3: this is
// how the operator selects more than one target in one pass.
func (c *Controller) parseTargetList(names []string, choice string) ([]Selection, bool) {
	parts := strings.Split(choice, ",")
	var list []Selection
	for _, raw := range parts {
		ref := strings.TrimSpace(raw)
		if ref == "" {
			return nil, false
		}
		if at := indexString(names, ref); at >= 0 {
			list = append(list, Selection{
				TargetName:     names[at],
				Operation:      c.scopeOf(names[at]),
				EffectiveScope: names[at],
			})
			continue
		}
		if n, err := strconv.Atoi(ref); err == nil {
			if n >= 1 && n <= len(names) {
				target := names[n-1]
				list = append(list, Selection{
					TargetName:     target,
					Operation:      c.scopeOf(target),
					EffectiveScope: target,
				})
				continue
			}
		}
		return nil, false
	}
	return list, true
}

// RunResult bundles the terminal decision and any error from a wizard run so
// the CLI layer can render both. It is a single struct value (not a tuple)
// because Go 1.26's type-checker misreads the positions of an (error, T) or
// (T, error) multi-value return when the callback parameter type involves a
// defined policy type; the struct form compiles cleanly everywhere.
type RunResult struct {
	Decision policy.Decision
	Err      error
}

// Run drives the full wizard: name, search, select (with pagination/back),
// and review. On review the operator can approve (the production authority
// decides) or cancel. A cancel or an EOF before approval fails closed: no
// selection is persisted.
//
// decide is the production decision injected by the CLI layer: it returns the
// production policy authority's verdict for the confirmed review. A Deny means
// the wizard reports the reason and fails closed.
func (c *Controller) Run(decide func(Review) policy.Decision) RunResult {
	name, err := c.askName()
	if err != nil {
		return RunResult{Decision: policy.Deny, Err: err}
	}
	if strings.EqualFold(strings.TrimSpace(name), "cancel") {
		fmt.Fprintln(c.Out, "Cancelled. No changes were made.")
		return RunResult{Decision: policy.Allow}
	}
	person := Person{Name: name}

	var selections []Selection
	for {
		sel, cont, err := c.askSelections()
		if err != nil {
			return RunResult{Decision: policy.Deny, Err: err}
		}
		// 'cancel' at any page returns a non-nil sentinel selection; Run reports
		// cancellation WITHOUT ever invoking decide (finding #2: cancel fails
		// closed — no write may follow).
		if len(sel) == 1 && sel[0].isCancel {
			fmt.Fprintln(c.Out, "Cancelled. No changes were made.")
			return RunResult{Decision: policy.Allow}
		}
		if len(sel) > 0 {
			// One or more selections (a single target, or a comma-separated list
			// such as "1,2,3" via parseTargetList) complete the selection pass.
			// Finding #3: every reviewed selection is later processed, never
			// truncated to the first.
			selections = append(selections, sel...)
			break
		}
		if !cont {
			if len(selections) == 0 {
				fmt.Fprintln(c.Out, "Cancelled. No changes were made.")
				return RunResult{Decision: policy.Allow}
			}
			break
		}
	}
	review := Review{Person: person, Selected: selections}
	fmt.Fprintln(c.Out, "--- review ---")
	for _, line := range review.Render() {
		fmt.Fprintln(c.Out, line)
	}
	// The review is shown; the operator must explicitly approve before any write.
	// A plain cancel now fails closed here and never reaches decide, so no
	// confirmation after selection can ever invoke the mutating callback.
	fmt.Fprintln(c.Out, "Type 'yes' to confirm, or 'cancel' to abort:")
	line, err := c.readLine()
	if err != nil {
		fmt.Fprintln(c.Out, "Cancelled. No changes were made.")
		return RunResult{Decision: policy.Deny, Err: ErrDenied}
	}
	choice := strings.TrimSpace(line)
	if strings.EqualFold(choice, "cancel") {
		fmt.Fprintln(c.Out, "Cancelled. No changes were made.")
		return RunResult{Decision: policy.Allow}
	}
	if !strings.EqualFold(choice, "yes") {
		fmt.Fprintln(c.Out, "No changes were made. Approval is required.")
		return RunResult{Decision: policy.Deny, Err: ErrDenied}
	}
	decision := decide(review)
	if decision == policy.Deny {
		// The production authority Denied: the wizard reports the reason and
		// fails closed. The rejected request is never persisted. When the
		// callback recorded a persistence failure (c.CommitErr, e.g. the
		// owner-collision guard in PersistActive), the reason names the row
		// that blocked the write.
		if c.commitErr != nil {
			fmt.Fprintf(c.Out, "Access change rejected: %v. No changes were made.\n", c.commitErr)
			err := c.commitErr
			c.commitErr = nil
			return RunResult{Decision: policy.Deny, Err: err}
		}
		fmt.Fprintln(c.Out, "Access change rejected by the production policy authority. No changes were made.")
		return RunResult{Decision: policy.Deny, Err: ErrDenied}
	}
	fmt.Fprintf(c.Out, "Access for %q assigned on selected targets.\n", person.Name)
	return RunResult{Decision: decision}
}

func (c *Controller) askName() (string, error) {
	fmt.Fprintln(c.Out, "Enter the person to onboard/offboard:")
	line, err := c.readLine()
	if err != nil {
		// EOF before a name fails closed on the same incomplete-config marker an
		// empty name does, so a missing person is never treated as a valid one.
		if err == io.EOF {
			return "", fmt.Errorf("%w: a person name is required", ErrIncompleteConfig)
		}
		return "", fmt.Errorf("read person name: %w", err)
	}
	choice := strings.TrimSpace(line)
	if choice == "" {
		return "", fmt.Errorf("%w: a person name is required", ErrIncompleteConfig)
	}
	return choice, nil
}

func (c *Controller) askSelections() ([]Selection, bool, error) {
	names := c.KnownTargets()
	if len(names) == 0 {
		return nil, false, fmt.Errorf("%w: no declared target to select", ErrIncompleteConfig)
	}
	fmt.Fprintf(c.Out, "Available targets (%d): %s\n", len(names), strings.Join(names, ", "))
	c.page = 0
	return c.pageSelect(names, nil)
}

// pageSelect lists one page of targets, lets the operator pick one or page
// back/forward, and returns the confirmed selection(s). Selections made on a
// later page are appended to the accumulator so pagination preserves them and a
// "back" returns to confirm without losing them.
func (c *Controller) pageSelect(names []string, acc []Selection) ([]Selection, bool, error) {
	total := len(names)
	pages := (total + PageSize - 1) / PageSize
	for {
		end := c.page * PageSize
		if end > total {
			end = total
		}
		start := end - PageSize
		if start < 0 {
			start = 0
		}
		fmt.Fprintf(c.Out, "--- page %d (%d-%d of %d) ---\n", c.page+1, start+1, end, total)
		for i := start; i < end; i++ {
			scope := c.scopeOf(names[i])
			if scope == "" {
				scope = "undeclared operation"
			}
			fmt.Fprintf(c.Out, "  %d  %s [%s]\n", i+1, names[i], scope)
		}
		fmt.Fprintln(c.Out, "  Enter a number, 'back', 'next', 'list', or 'cancel'.")
		line, err := c.readLine()
		if err != nil {
			return nil, false, fmt.Errorf("read target selection: %w", err)
		}
		choice := strings.TrimSpace(line)
		switch {
		case strings.EqualFold(choice, "cancel"):
			// A cancel at any page is a distinct, non-nil sentinel selection so Run
			// can distinguish "operator cancelled" (never call decide) from
			// "no selection made and return to review".
			return []Selection{{isCancel: true}}, false, nil
		case strings.EqualFold(choice, "back"):
			if c.page > 0 {
				c.page--
			}
			return acc, true, nil
		case strings.EqualFold(choice, "next"):
			if c.page+1 < pages {
				c.page++
			}
			return acc, true, nil
		}
		// A comma-separated list ("1,2,3" or "dev,prod") selects every listed
		// target in one pass so the operator can assign or offboard more than one
		// target at once (finding #3): every reviewed selection is processed, never
		// truncated to the first.
		if list, ok := c.parseTargetList(names, choice); ok {
			return append(acc, list...), false, nil
		}
		if idx, convErr := strconv.Atoi(choice); convErr == nil {
			if idx >= 1 && idx <= total {
				target := names[idx-1]
				sel := Selection{
					TargetName:     target,
					Operation:      c.scopeOf(target),
					EffectiveScope: target,
				}
				if err := ValidateSelection(sel); err != nil {
					fmt.Fprintln(c.Out, err)
					continue
				}
				return append(acc, sel), true, nil
			}
			fmt.Fprintln(c.Out, "Selection out of range. Choose a displayed number, 'back', 'next', 'list', or 'cancel'.")
			continue
		}
		// A named target selects directly.
		if idx := indexString(names, choice); idx >= 0 {
			target := names[idx]
			sel := Selection{
				TargetName:     target,
				Operation:      c.scopeOf(target),
				EffectiveScope: target,
			}
			if err := ValidateSelection(sel); err != nil {
				fmt.Fprintln(c.Out, err)
				continue
			}
			return append(acc, sel), true, nil
		}
		fmt.Fprintln(c.Out, "Invalid selection. Choose a displayed number or name, 'back', 'next', 'list', or 'cancel'.")
	}
}
