"""Constraint serialization and basic policy derivation."""

from __future__ import annotations

import json
from typing import Any

from .model import OutputConstraint


def constraint_to_document(constraint: OutputConstraint) -> dict[str, Any]:
    return {
        "expression": constraint.expression,
        "name": constraint.name,
    }


def constraints_to_documents(
    constraints: tuple[OutputConstraint, ...] | list[OutputConstraint],
) -> list[dict[str, Any]]:
    docs = [constraint_to_document(constraint) for constraint in constraints]
    return sorted(
        docs,
        key=lambda doc: json.dumps(doc, sort_keys=True, separators=(",", ":")),
    )


def constraint_from_document(doc: dict[str, Any]) -> OutputConstraint:
    name = doc.get("name")
    expression = doc.get("expression")
    if not isinstance(name, str) or not name:
        raise ValueError(f"constraint name must be a non-empty string: {doc!r}")
    if not isinstance(expression, str) or not expression:
        raise ValueError(
            f"constraint expression must be a non-empty string: {doc!r}"
        )
    return OutputConstraint(name=name, expression=expression)


def constraints_from_documents(documents: list[dict[str, Any]]) -> tuple[OutputConstraint, ...]:
    return tuple(constraint_from_document(document) for document in documents)


def signed_input_envelope(
    content: Any,
    valid_until: float,
    constraints: tuple[OutputConstraint, ...],
) -> dict[str, Any]:
    return {
        "content": content,
        "output_constraints": constraints_to_documents(constraints),
        "valid_until": valid_until,
    }


def derive_output_constraints(content: Any) -> tuple[OutputConstraint, ...]:
    if not isinstance(content, dict):
        return ()

    strategy = content.get("manifest_strategy", {})
    if not isinstance(strategy, dict):
        return ()

    if strategy.get("type") != "addon":
        return ()

    addon_id = strategy.get("addon_id") or strategy.get("addon")
    if not addon_id:
        return ()

    trust_anchor_id = (
        strategy.get("trust_anchor_id")
        or strategy.get("trust_anchor")
        or "fleet-addons"
    )
    return (
        OutputConstraint(
            name=f"output must be signed by {addon_id} via {trust_anchor_id}",
            expression=(
                f'output.has_signature && '
                f'output.signature.trust_anchor_id == "{trust_anchor_id}" && '
                f'output.signer_id == "{addon_id}"'
            ),
        ),
    )


def describe_constraint(constraint: OutputConstraint) -> str:
    return constraint.name
