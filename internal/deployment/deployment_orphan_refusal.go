package deployment

import "errors"

// ErrTargetDetachedWithOrphans is returned when a Target definition is detached
// while one or more deployment records remain under its (now ownerless)
// deployments namespace. Detach is config-only by contract, but the orphaned
// records are otherwise uncollectable: the one-installation-owner seam
// (VerifyDeploymentOwner) fails closed on an ownerless Target, so neither
// `app destroy --uuid --yes` nor `destroy --all` can resolve them and no
// later operation reaches them. The detach is refused instead, naming the
// records, so the operator can restore the authoritative Core installation owner
// of the Target (and the source repository/manifest that owns each record) and
// run `baha app destroy` normally against the still-attached records. If those
// prerequisites cannot be restored, there is no supported cleanup command and
// the records must remain attached for manual operator recovery. This is a
// visible, actionable refusal — never a silent removal — and it keeps the
// ErrDeploymentOwnerMissing boundary intact.
var ErrTargetDetachedWithOrphans = errors.New(
	"Target has deployment records that would become uncollectable on detach; restore the Target and collect them first",
)
