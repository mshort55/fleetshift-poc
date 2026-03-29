"""
Tests for the Attestation (input + output) model.

Every attestation is the same shape: input + output.  Inputs carry
constraints; outputs don't.  DerivedInput is a kind of input, not
a kind of attestation.

Scenarios:
  1.  SignedInput verification (standalone)
  2.  Direct signing (user signs manifests, unsigned output)
  3.  Addon signing (user spec, addon-signed output)
  4.  Constraint violations (wrong addon, namespace, cluster-admin)
  5.  CEL update inline (DerivedInput, unsigned output)
  6.  CEL update tampered (derivation fails)
  7.  CEL update + addon (DerivedInput, addon-signed output)
  8.  Addon produces patch (planner addon as update source)
  9.  Constraint propagation through derivation
  10. Hard case: D1v1 + D2 -> D1v2 (cross-deployment update)
  11. Chained updates (D1v1 -> D1v2 -> D1v3)
  12. Sub-attestation failures propagate
  13. Signature failures (expired, forged, untrusted, tampered)
  14. Output signature failures
  15. Multiple constraints (GVK + namespace)

Run: pytest test_attestation.py -v
"""

from __future__ import annotations

import copy
import time
from dataclasses import dataclass

import pytest

from attestation import (
    Attestation,
    DerivedInput,
    KeyBinding,
    KeyPair,
    Output,
    OutputConstraint,
    SignedInput,
    TrustAnchor,
    TrustStore,
    VerificationError,
    VerifiedOutput,
    generate_keypair,
    make_key_binding,
    make_output,
    make_signed_input,
    sign_output,
)


# ---------------------------------------------------------------------------
# Test identity helper
# ---------------------------------------------------------------------------

@dataclass
class Identity:
    signer_id: str
    keys: KeyPair
    key_binding: KeyBinding


def make_identity(signer_id: str, trust_anchor_id: str) -> Identity:
    keys = generate_keypair()
    kb = make_key_binding(keys, signer_id, trust_anchor_id)
    return Identity(signer_id=signer_id, keys=keys, key_binding=kb)


def _input(identity: Identity, content, **kwargs) -> SignedInput:
    return make_signed_input(identity.keys, identity.key_binding, content, **kwargs)


def _output(identity: Identity, content) -> Output:
    return sign_output(identity.keys, identity.key_binding, content)


# ---------------------------------------------------------------------------
# Output constraint helpers
# ---------------------------------------------------------------------------

def addon_must_sign(expected_addon_id: str) -> OutputConstraint:
    return OutputConstraint(
        description=f"output must be signed by {expected_addon_id}",
        check=lambda _auth, out: out.signer_id == expected_addon_id,
    )


def namespace_constraint(ns: str) -> OutputConstraint:
    return OutputConstraint(
        description=f"all manifests must be in namespace '{ns}'",
        check=lambda _auth, out: all(
            m.get("metadata", {}).get("namespace") == ns
            for m in out.content
        ),
    )


def no_cluster_admin() -> OutputConstraint:
    return OutputConstraint(
        description="no ClusterRoleBinding granting cluster-admin",
        check=lambda _auth, out: all(
            not (
                m.get("kind") == "ClusterRoleBinding"
                and m.get("roleRef", {}).get("name") == "cluster-admin"
            )
            for m in out.content
        ),
    )


def allowed_gvks(allowed: set[str]) -> OutputConstraint:
    def check(_auth, out: VerifiedOutput) -> bool:
        for m in out.content:
            gvk = f"{m.get('apiVersion', '')}/{m.get('kind', '')}"
            if gvk not in allowed:
                return False
        return True

    return OutputConstraint(description=f"only GVKs in {allowed}", check=check)


# ---------------------------------------------------------------------------
# Derivation helpers
# ---------------------------------------------------------------------------

def cel_set_version(version: str):
    """Return a function that sets manifest_strategy.config.version."""
    def transform(input_spec: dict) -> dict:
        out = copy.deepcopy(input_spec)
        out.setdefault("manifest_strategy", {}).setdefault("config", {})["version"] = version
        return out
    return transform


def cel_upgrade_apply(prior_content, update_content):
    """Apply a CEL-like mutation: update_content specifies the new version."""
    version = update_content.get("new_version")
    if not version:
        raise ValueError("update_content must specify new_version")
    new_spec = cel_set_version(version)(prior_content)
    constraints = _derive_constraints(new_spec)
    return new_spec, constraints


def _derive_constraints(spec: dict) -> list[OutputConstraint]:
    """Domain logic: derive output constraints implied by a spec."""
    constraints: list[OutputConstraint] = []
    ms = spec.get("manifest_strategy", {})
    if ms.get("type") == "addon":
        constraints.append(addon_must_sign(ms["addon_id"]))
    return constraints


# ---------------------------------------------------------------------------
# Fixtures
# ---------------------------------------------------------------------------

@pytest.fixture
def alice():
    return make_identity("alice", "tenant-idp")


@pytest.fixture
def bob():
    return make_identity("bob", "tenant-idp")


@pytest.fixture
def capi_addon():
    return make_identity("capi-provisioner", "fleet-addons")


@pytest.fixture
def lifecycle_addon():
    return make_identity("cluster-lifecycle", "fleet-addons")


@pytest.fixture
def planner_addon():
    return make_identity("upgrade-planner", "fleet-addons")


@pytest.fixture
def trust_store(alice, bob, capi_addon, lifecycle_addon, planner_addon):
    ts = TrustStore()
    ts.add(TrustAnchor(
        anchor_id="tenant-idp",
        known_keys={
            "alice": alice.keys.public_key_bytes,
            "bob": bob.keys.public_key_bytes,
        },
    ))
    ts.add(TrustAnchor(
        anchor_id="fleet-addons",
        known_keys={
            "capi-provisioner": capi_addon.keys.public_key_bytes,
            "cluster-lifecycle": lifecycle_addon.keys.public_key_bytes,
            "upgrade-planner": planner_addon.keys.public_key_bytes,
        },
    ))
    return ts


# ===================================================================
# 1. SignedInput verification
# ===================================================================

class TestSignedInput:
    def test_valid(self, alice, trust_store):
        si = _input(alice, {"version": "1.29"})
        result = si.verify(trust_store)
        assert result.content == {"version": "1.29"}
        assert result.signer_id == "alice"
        assert result.output_constraints == []

    def test_with_constraints(self, alice, trust_store):
        si = _input(
            alice, {"manifest_strategy": {"type": "addon"}},
            output_constraints=[addon_must_sign("capi-provisioner")],
        )
        result = si.verify(trust_store)
        assert len(result.output_constraints) == 1


# ===================================================================
# 2. Direct signing (unsigned output)
# ===================================================================

class TestDirectSigning:
    def test_valid(self, alice, trust_store):
        manifests = [
            {"apiVersion": "v1", "kind": "Secret",
             "metadata": {"name": "db-creds", "namespace": "production"},
             "data": {"password": "c2VjcmV0"}},
        ]
        att = Attestation(
            input=_input(alice, manifests),
            output=make_output(manifests),
        )
        result = att.verify(trust_store)
        assert result.content == manifests
        assert result.signer_id is None

    def test_with_self_constraints(self, alice, trust_store):
        manifests = [
            {"kind": "Deployment",
             "metadata": {"name": "web", "namespace": "prod"}},
        ]
        att = Attestation(
            input=_input(
                alice, manifests,
                output_constraints=[namespace_constraint("prod")],
            ),
            output=make_output(manifests),
        )
        result = att.verify(trust_store)
        assert result.content == manifests


# ===================================================================
# 3. Addon signing (signed output)
# ===================================================================

class TestAddonSigning:
    def test_valid(self, alice, capi_addon, trust_store):
        spec = {
            "manifest_strategy": {
                "type": "addon", "addon_id": "capi-provisioner",
            },
            "placement": {"selector": {"pool": "management"}},
        }
        manifests = [
            {"apiVersion": "cluster.x-k8s.io/v1beta1", "kind": "Cluster",
             "metadata": {"name": "workload-01", "namespace": "capi-system"},
             "spec": {"topology": {"version": "v1.29.5"}}},
        ]

        att = Attestation(
            input=_input(
                alice, spec,
                output_constraints=[
                    addon_must_sign("capi-provisioner"),
                    no_cluster_admin(),
                ],
            ),
            output=_output(capi_addon, manifests),
        )
        result = att.verify(trust_store)
        assert result.content == manifests
        assert result.signer_id == "capi-provisioner"


# ===================================================================
# 4. Constraint violations
# ===================================================================

class TestConstraintViolations:
    def test_unauthorized_addon(self, alice, lifecycle_addon, trust_store):
        spec = {
            "manifest_strategy": {
                "type": "addon", "addon_id": "capi-provisioner",
            },
        }
        manifests = [{"kind": "ConfigMap", "metadata": {"name": "x"}}]

        att = Attestation(
            input=_input(
                alice, spec,
                output_constraints=[addon_must_sign("capi-provisioner")],
            ),
            output=_output(lifecycle_addon, manifests),
        )
        with pytest.raises(
            VerificationError,
            match="output constraint failed.*signed by capi-provisioner",
        ):
            att.verify(trust_store)

    def test_namespace_violation(self, alice, capi_addon, trust_store):
        spec = {"manifest_strategy": {"type": "addon"}}
        wrong_ns = [
            {"kind": "Deployment",
             "metadata": {"name": "web", "namespace": "kube-system"}},
        ]

        att = Attestation(
            input=_input(
                alice, spec,
                output_constraints=[
                    addon_must_sign("capi-provisioner"),
                    namespace_constraint("production"),
                ],
            ),
            output=_output(capi_addon, wrong_ns),
        )
        with pytest.raises(VerificationError, match="namespace"):
            att.verify(trust_store)

    def test_cluster_admin_blocked(self, alice, capi_addon, trust_store):
        spec = {"manifest_strategy": {"type": "addon"}}
        evil = [
            {"apiVersion": "rbac.authorization.k8s.io/v1",
             "kind": "ClusterRoleBinding",
             "metadata": {"name": "evil"},
             "roleRef": {"name": "cluster-admin"}},
        ]

        att = Attestation(
            input=_input(
                alice, spec,
                output_constraints=[
                    addon_must_sign("capi-provisioner"),
                    no_cluster_admin(),
                ],
            ),
            output=_output(capi_addon, evil),
        )
        with pytest.raises(VerificationError, match="cluster-admin"):
            att.verify(trust_store)


# ===================================================================
# 5. CEL update inline (DerivedInput, unsigned output)
# ===================================================================

class TestCelUpdateInline:
    def test_valid(self, alice, trust_store):
        old_spec = {
            "deployment_id": "cluster-deploy-007",
            "manifest_strategy": {
                "type": "inline",
                "config": {"version": "1.29.5", "region": "us-east-1"},
            },
        }
        update_content = {
            "type": "update",
            "cel_expression": 'spec.manifest_strategy.config.version = "1.30.2"',
            "new_version": "1.30.2",
        }

        def inline_apply(prior, update):
            return cel_set_version(update["new_version"])(prior), []

        new_spec = cel_set_version("1.30.2")(old_spec)

        att = Attestation(
            input=DerivedInput(
                prior=_input(alice, old_spec),
                update=Attestation(
                    input=_input(alice, update_content),
                    output=make_output(update_content),
                ),
                apply=inline_apply,
            ),
            output=make_output(new_spec),
        )
        result = att.verify(trust_store)
        assert result.content["manifest_strategy"]["config"]["version"] == "1.30.2"
        assert result.content["manifest_strategy"]["config"]["region"] == "us-east-1"


# ===================================================================
# 6. CEL update tampered
# ===================================================================

class TestCelUpdateTampered:
    def test_derivation_mismatch_detected(self, alice, trust_store):
        old_spec = {
            "manifest_strategy": {
                "type": "inline",
                "config": {"version": "1.29.5"},
            },
        }
        update_content = {"type": "update", "new_version": "1.30.2"}

        def tampered_apply(prior, update):
            raise ValueError("mutation does not match signed expression")

        att = Attestation(
            input=DerivedInput(
                prior=_input(alice, old_spec),
                update=Attestation(
                    input=_input(alice, update_content),
                    output=make_output(update_content),
                ),
                apply=tampered_apply,
            ),
            output=make_output({"whatever": True}),
        )
        with pytest.raises(VerificationError, match="derivation failed"):
            att.verify(trust_store)


# ===================================================================
# 7. CEL update + addon (DerivedInput, addon-signed output)
# ===================================================================

class TestCelUpdateWithAddon:
    def test_valid(self, alice, capi_addon, trust_store):
        old_spec = {
            "manifest_strategy": {
                "type": "addon",
                "addon_id": "capi-provisioner",
                "config": {"version": "1.29.5"},
            },
        }
        update_content = {
            "type": "update",
            "cel_expression": 'spec.config.version = "1.30.2"',
            "new_version": "1.30.2",
        }
        manifests_v2 = [
            {"apiVersion": "cluster.x-k8s.io/v1beta1", "kind": "Cluster",
             "metadata": {"name": "workload-010", "namespace": "capi-system"},
             "spec": {"topology": {"version": "v1.30.2"}}},
        ]

        att = Attestation(
            input=DerivedInput(
                prior=_input(
                    alice, old_spec,
                    output_constraints=[addon_must_sign("capi-provisioner")],
                ),
                update=Attestation(
                    input=_input(alice, update_content),
                    output=make_output(update_content),
                ),
                apply=cel_upgrade_apply,
            ),
            output=_output(capi_addon, manifests_v2),
        )

        result = att.verify(trust_store)
        assert result.content == manifests_v2
        assert result.signer_id == "capi-provisioner"


# ===================================================================
# 8. Addon produces patch (planner addon as update source)
# ===================================================================

class TestAddonProducesPatch:
    def test_valid(self, alice, planner_addon, trust_store):
        old_spec = {
            "deployment_id": "cluster-deploy-020",
            "manifest_strategy": {
                "type": "inline",
                "config": {"version": "1.29.5", "region": "us-east-1"},
            },
        }
        intent = {
            "type": "update",
            "capability": "upgrade-planner",
        }
        patch = {
            "type": "version_patch",
            "target_field": "manifest_strategy.config.version",
            "new_version": "1.30.2",
        }

        def apply_patch(prior, update):
            new = cel_set_version(update["new_version"])(prior)
            return new, []

        new_spec = cel_set_version("1.30.2")(old_spec)

        att = Attestation(
            input=DerivedInput(
                prior=_input(alice, old_spec),
                update=Attestation(
                    input=_input(
                        alice, intent,
                        output_constraints=[addon_must_sign("upgrade-planner")],
                    ),
                    output=_output(planner_addon, patch),
                ),
                apply=apply_patch,
            ),
            output=make_output(new_spec),
        )
        result = att.verify(trust_store)
        assert result.content["manifest_strategy"]["config"]["version"] == "1.30.2"
        assert result.content["manifest_strategy"]["config"]["region"] == "us-east-1"

    def test_wrong_planner(self, alice, lifecycle_addon, trust_store):
        """User expects the upgrade-planner but a different addon signs."""
        old_spec = {"manifest_strategy": {"config": {"version": "1.29.5"}}}
        intent = {"type": "update", "capability": "upgrade-planner"}
        patch = {"type": "version_patch", "new_version": "1.30.2"}

        def apply_patch(prior, update):
            return cel_set_version(update["new_version"])(prior), []

        att = Attestation(
            input=DerivedInput(
                prior=_input(alice, old_spec),
                update=Attestation(
                    input=_input(
                        alice, intent,
                        output_constraints=[addon_must_sign("upgrade-planner")],
                    ),
                    output=_output(lifecycle_addon, patch),
                ),
                apply=apply_patch,
            ),
            output=make_output({"whatever": True}),
        )
        with pytest.raises(
            VerificationError,
            match="output constraint failed.*upgrade-planner",
        ):
            att.verify(trust_store)


# ===================================================================
# 9. Constraint propagation through derivation
# ===================================================================

class TestConstraintPropagation:
    def test_update_changes_addon_constraint(
        self, alice, capi_addon, lifecycle_addon, trust_store,
    ):
        """Update changes the addon; the derived constraint must follow."""
        old_spec = {
            "manifest_strategy": {
                "type": "addon",
                "addon_id": "capi-provisioner",
                "config": {"version": "1.29.5"},
            },
        }

        update_content = {"type": "update", "new_version": "1.30.2"}

        def upgrade_and_switch_addon(prior, update):
            new = cel_set_version(update["new_version"])(prior)
            new["manifest_strategy"]["addon_id"] = "cluster-lifecycle"
            return new, _derive_constraints(new)

        derived_input = DerivedInput(
            prior=_input(
                alice, old_spec,
                output_constraints=[addon_must_sign("capi-provisioner")],
            ),
            update=Attestation(
                input=_input(alice, update_content),
                output=make_output(update_content),
            ),
            apply=upgrade_and_switch_addon,
        )

        manifests_v2 = [{"kind": "SomeNewResource"}]

        wrong = Attestation(input=derived_input, output=_output(capi_addon, manifests_v2))
        with pytest.raises(
            VerificationError,
            match="output constraint failed.*cluster-lifecycle",
        ):
            wrong.verify(trust_store)

        right = Attestation(input=derived_input, output=_output(lifecycle_addon, manifests_v2))
        result = right.verify(trust_store)
        assert result.content == manifests_v2
        assert result.signer_id == "cluster-lifecycle"


# ===================================================================
# 10. Hard case: D1v1 + D2 -> D1v2 (cross-deployment update)
# ===================================================================

class TestCrossDeploymentUpdate:
    """
    D1v1: UserA spec -> AddonX manifests_v1
    D2:   UserB spec -> AddonY update_instruction
    D1v2: DerivedInput(prior=D1_input, update=D2) -> AddonX manifests_v2

    D2 is a full Attestation used as the update.  Its verified
    output (the update instruction) feeds into the derivation.
    """

    def test_full_scenario(
        self, alice, bob, capi_addon, lifecycle_addon, trust_store,
    ):
        d1_spec = {
            "deployment_id": "dep-001",
            "manifest_strategy": {
                "type": "addon",
                "addon_id": "capi-provisioner",
                "config": {"version": "1.29.5"},
            },
            "placement": {"selector": {"pool": "production"}},
        }
        manifests_v1 = [
            {"apiVersion": "cluster.x-k8s.io/v1beta1", "kind": "Cluster",
             "metadata": {"name": "prod-001", "namespace": "capi-system"},
             "spec": {"topology": {"version": "v1.29.5"}}},
        ]

        d1_input = _input(
            alice, d1_spec,
            output_constraints=[addon_must_sign("capi-provisioner")],
        )
        d1v1 = Attestation(input=d1_input, output=_output(capi_addon, manifests_v1))

        d2_spec = {
            "type": "fleet-update",
            "description": "upgrade all production clusters to 1.30.2",
            "target_selector": {"labels": {"pool": "production"}},
        }
        update_instruction = {
            "type": "deployment-update",
            "new_version": "1.30.2",
            "target_selector": {"labels": {"pool": "production"}},
        }

        d2 = Attestation(
            input=_input(
                bob, d2_spec,
                output_constraints=[addon_must_sign("cluster-lifecycle")],
            ),
            output=_output(lifecycle_addon, update_instruction),
        )

        manifests_v2 = [
            {"apiVersion": "cluster.x-k8s.io/v1beta1", "kind": "Cluster",
             "metadata": {"name": "prod-001", "namespace": "capi-system"},
             "spec": {"topology": {"version": "v1.30.2"}}},
        ]

        d1v2 = Attestation(
            input=DerivedInput(
                prior=d1_input,
                update=d2,
                apply=cel_upgrade_apply,
            ),
            output=_output(capi_addon, manifests_v2),
        )

        result = d1v2.verify(trust_store)
        assert result.content == manifests_v2
        assert result.signer_id == "capi-provisioner"

    def test_d1v1_verifies_independently(
        self, alice, capi_addon, trust_store,
    ):
        d1_spec = {
            "manifest_strategy": {
                "type": "addon",
                "addon_id": "capi-provisioner",
                "config": {"version": "1.29.5"},
            },
        }
        manifests_v1 = [{"kind": "Cluster"}]

        d1_input = _input(
            alice, d1_spec,
            output_constraints=[addon_must_sign("capi-provisioner")],
        )
        d1v1 = Attestation(input=d1_input, output=_output(capi_addon, manifests_v1))

        result = d1v1.verify(trust_store)
        assert result.content == manifests_v1
        assert result.signer_id == "capi-provisioner"


# ===================================================================
# 11. Chained updates (D1v1 -> D1v2 -> D1v3)
# ===================================================================

class TestChainedUpdates:
    def test_three_version_chain(self, alice, capi_addon, trust_store):
        d1_spec = {
            "manifest_strategy": {
                "type": "addon",
                "addon_id": "capi-provisioner",
                "config": {"version": "1.28"},
            },
        }

        d1_input = _input(
            alice, d1_spec,
            output_constraints=[addon_must_sign("capi-provisioner")],
        )

        def make_update_att(version):
            content = {"type": "update", "new_version": version}
            return Attestation(
                input=_input(alice, content),
                output=make_output(content),
            )

        # v2: 1.28 -> 1.29
        d1v2_input = DerivedInput(
            prior=d1_input,
            update=make_update_att("1.29"),
            apply=cel_upgrade_apply,
        )

        # v3: 1.29 -> 1.30 (prior is the previous DerivedInput)
        d1v3_input = DerivedInput(
            prior=d1v2_input,
            update=make_update_att("1.30"),
            apply=cel_upgrade_apply,
        )

        manifests_v3 = [{"kind": "Cluster", "spec": {"version": "1.30"}}]
        d1v3 = Attestation(
            input=d1v3_input,
            output=_output(capi_addon, manifests_v3),
        )

        result = d1v3.verify(trust_store)
        assert result.content == manifests_v3
        assert result.signer_id == "capi-provisioner"


# ===================================================================
# 12. Sub-attestation failures propagate
# ===================================================================

class TestSubAttestationFailures:
    def test_forged_addon_in_update(
        self, alice, bob, capi_addon, lifecycle_addon, trust_store,
    ):
        """Forged addon signature in D2 causes D1v2 verification to fail."""
        d1_spec = {
            "manifest_strategy": {
                "type": "addon",
                "addon_id": "capi-provisioner",
                "config": {"version": "1.29.5"},
            },
        }
        d1_input = _input(
            alice, d1_spec,
            output_constraints=[addon_must_sign("capi-provisioner")],
        )

        forger = make_identity("cluster-lifecycle", "fleet-addons")
        update = {"type": "deployment-update", "new_version": "1.30.2"}
        d2 = Attestation(
            input=_input(
                bob, {"type": "fleet-update"},
                output_constraints=[addon_must_sign("cluster-lifecycle")],
            ),
            output=_output(forger, update),
        )

        manifests_v2 = [{"kind": "Cluster"}]
        d1v2 = Attestation(
            input=DerivedInput(prior=d1_input, update=d2, apply=cel_upgrade_apply),
            output=_output(capi_addon, manifests_v2),
        )

        with pytest.raises(VerificationError, match="key not recognised"):
            d1v2.verify(trust_store)

    def test_untrusted_update_signer(self, alice, capi_addon, trust_store):
        """Update signed by unknown user -> fails."""
        d1_spec = {
            "manifest_strategy": {
                "type": "addon",
                "addon_id": "capi-provisioner",
                "config": {"version": "1.29.5"},
            },
        }
        d1_input = _input(
            alice, d1_spec,
            output_constraints=[addon_must_sign("capi-provisioner")],
        )

        eve = make_identity("eve", "rogue-idp")
        update = {"type": "deployment-update", "new_version": "9.9.9"}

        d1v2 = Attestation(
            input=DerivedInput(
                prior=d1_input,
                update=Attestation(
                    input=_input(eve, update),
                    output=make_output(update),
                ),
                apply=cel_upgrade_apply,
            ),
            output=_output(capi_addon, [{"kind": "Cluster"}]),
        )

        with pytest.raises(VerificationError, match="trust anchor not found"):
            d1v2.verify(trust_store)


# ===================================================================
# 13. Signature failures
# ===================================================================

class TestSignatureFailures:
    def test_expired(self, alice, trust_store):
        si = _input(alice, {"x": 1}, valid_duration_sec=-1)
        with pytest.raises(VerificationError, match="expired"):
            si.verify(trust_store)

    def test_forged(self, alice, trust_store):
        si = _input(alice, {"x": 1})
        forger_keys = generate_keypair()
        forger_kb = make_key_binding(forger_keys, "alice", "tenant-idp")
        forged = make_signed_input(forger_keys, forger_kb, {"x": 1})

        tampered = SignedInput(
            content=si.content,
            signer_id=si.signer_id,
            public_key=si.public_key,
            signature=forged.signature,
            valid_until=si.valid_until,
            key_binding=si.key_binding,
            output_constraints=si.output_constraints,
        )
        with pytest.raises(VerificationError, match="signature verification failed"):
            tampered.verify(trust_store)

    def test_untrusted_signer(self, alice):
        empty_store = TrustStore()
        si = _input(alice, {"x": 1})
        with pytest.raises(VerificationError, match="trust anchor not found"):
            si.verify(empty_store)

    def test_key_not_in_anchor(self, trust_store):
        unknown = make_identity("eve", "tenant-idp")
        si = _input(unknown, {"x": 1})
        with pytest.raises(VerificationError, match="key not recognised"):
            si.verify(trust_store)

    def test_tampered_valid_until(self, alice, trust_store):
        si = _input(alice, {"x": 1}, valid_duration_sec=-1)
        tampered = SignedInput(
            content=si.content,
            signer_id=si.signer_id,
            public_key=si.public_key,
            signature=si.signature,
            valid_until=time.time() + 86400,
            key_binding=si.key_binding,
            output_constraints=si.output_constraints,
        )
        with pytest.raises(VerificationError, match="signature verification failed"):
            tampered.verify(trust_store)

    def test_tampered_constraints(self, alice, trust_store):
        si = _input(
            alice, {"x": 1},
            output_constraints=[namespace_constraint("prod")],
        )
        tampered = SignedInput(
            content=si.content,
            signer_id=si.signer_id,
            public_key=si.public_key,
            signature=si.signature,
            valid_until=si.valid_until,
            key_binding=si.key_binding,
            output_constraints=[],
        )
        with pytest.raises(VerificationError, match="signature verification failed"):
            tampered.verify(trust_store)

    def test_expiry_tampered_in_attestation(self, alice, trust_store):
        """Platform extends valid_until after signing. Signature breaks."""
        si = _input(alice, [{"kind": "ConfigMap"}], valid_duration_sec=-1)
        tampered = SignedInput(
            content=si.content,
            signer_id=si.signer_id,
            public_key=si.public_key,
            signature=si.signature,
            valid_until=time.time() + 86400,
            key_binding=si.key_binding,
            output_constraints=si.output_constraints,
        )
        att = Attestation(
            input=tampered,
            output=make_output([{"kind": "ConfigMap"}]),
        )
        with pytest.raises(VerificationError, match="signature verification failed"):
            att.verify(trust_store)


# ===================================================================
# 14. Output signature failures
# ===================================================================

class TestOutputSignatureFailures:
    def test_forged_output(self, alice, capi_addon, trust_store):
        spec = {"manifest_strategy": {"type": "addon"}}
        manifests = [{"kind": "Cluster"}]

        forger = make_identity("capi-provisioner", "fleet-addons")
        att = Attestation(
            input=_input(
                alice, spec,
                output_constraints=[addon_must_sign("capi-provisioner")],
            ),
            output=_output(forger, manifests),
        )
        with pytest.raises(VerificationError, match="key not recognised"):
            att.verify(trust_store)

    def test_tampered_output_content(self, alice, capi_addon, trust_store):
        spec = {"manifest_strategy": {"type": "addon"}}
        signed = _output(capi_addon, [{"kind": "Cluster"}])

        tampered_output = Output(
            content=[{"kind": "ClusterRoleBinding",
                      "roleRef": {"name": "cluster-admin"}}],
            signer_id=signed.signer_id,
            public_key=signed.public_key,
            signature=signed.signature,
            key_binding=signed.key_binding,
        )
        att = Attestation(
            input=_input(
                alice, spec,
                output_constraints=[addon_must_sign("capi-provisioner")],
            ),
            output=tampered_output,
        )
        with pytest.raises(VerificationError, match="output signature verification failed"):
            att.verify(trust_store)


# ===================================================================
# 15. Multiple constraints (GVK + namespace)
# ===================================================================

class TestMultipleConstraints:
    def test_all_constraints_pass(self, alice, trust_store):
        manifests = [
            {"apiVersion": "apps/v1", "kind": "Deployment",
             "metadata": {"name": "web", "namespace": "prod"}},
        ]

        att = Attestation(
            input=_input(
                alice, manifests,
                output_constraints=[
                    namespace_constraint("prod"),
                    allowed_gvks({"apps/v1/Deployment", "v1/Service"}),
                ],
            ),
            output=make_output(manifests),
        )
        result = att.verify(trust_store)
        assert result.content == manifests

    def test_gvk_violation(self, alice, trust_store):
        manifests = [
            {"apiVersion": "apps/v1", "kind": "DaemonSet",
             "metadata": {"name": "agent", "namespace": "prod"}},
        ]

        att = Attestation(
            input=_input(
                alice, manifests,
                output_constraints=[
                    allowed_gvks({"apps/v1/Deployment", "v1/Service"}),
                ],
            ),
            output=make_output(manifests),
        )
        with pytest.raises(VerificationError, match="output constraint failed"):
            att.verify(trust_store)
