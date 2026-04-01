"""
Hybrid attestation prototype.

Combines:
  - a single Attestation(input, output) model
  - explicit, signed CEL output constraints on inputs
  - self-contained verification bundles (no external store)
  - data-driven derivation from signed CEL update outputs
  - explainable verification results
"""

from .build import make_key_binding, make_output, make_signed_input, sign_output
from .cel_runtime import CelEvaluationError
from .crypto import KeyPair, generate_keypair
from .model import (
    Attestation,
    DerivedInput,
    KeyBinding,
    Output,
    OutputConstraint,
    OutputSignature,
    SignedInput,
    Signature,
    TrustAnchor,
    TrustAnchorConstraint,
    TrustAnchorSubject,
    VerifiedOutput,
)
from .policy import derive_output_constraints
from .verify import (
    TrustStore,
    VerificationBundle,
    VerificationError,
    VerificationResult,
    explain_verification,
    verify_attestation,
)

__all__ = [
    "Attestation",
    "CelEvaluationError",
    "DerivedInput",
    "KeyBinding",
    "KeyPair",
    "Output",
    "OutputConstraint",
    "OutputSignature",
    "Signature",
    "SignedInput",
    "TrustAnchor",
    "TrustAnchorConstraint",
    "TrustAnchorSubject",
    "TrustStore",
    "VerificationBundle",
    "VerificationError",
    "VerificationResult",
    "VerifiedOutput",
    "derive_output_constraints",
    "explain_verification",
    "generate_keypair",
    "make_key_binding",
    "make_output",
    "make_signed_input",
    "sign_output",
    "verify_attestation",
]
