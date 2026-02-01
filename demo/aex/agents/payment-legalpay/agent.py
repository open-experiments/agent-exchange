"""LegalPay - General legal payment processor with consistent fees."""

import logging
import os
from dataclasses import dataclass, field
from typing import Optional

import sys
sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

from common.payment_agent import BasePaymentAgent
from common.config import AgentConfig

logger = logging.getLogger(__name__)


@dataclass
class LegalPayAgent(BasePaymentAgent):
    """
    LegalPay - General Legal Payment Provider

    Characteristics:
    - Base fee: 2.0%
    - Rewards: 1% on ALL categories (consistent pricing)
    - Net fee: 1.0% on everything
    - Processing: Fast (3 seconds)
    - Fraud protection: Basic

    Best for:
    - Users who want predictable, consistent fees
    - Mixed legal work (contracts, compliance, research)
    - Those who value simplicity over optimization
    """

    # Payment provider characteristics
    base_fee_percent: float = 2.0
    processing_time_seconds: int = 3
    supported_methods: list[str] = field(default_factory=lambda: ["card", "aex_balance"])
    fraud_protection: str = "basic"

    # Category rewards - consistent 1% across all categories
    category_rewards: dict[str, float] = field(default_factory=lambda: {
        "contracts": 1.0,
        "contract_review": 1.0,
        "compliance": 1.0,
        "compliance_check": 1.0,
        "ip_patent": 1.0,
        "real_estate": 1.0,
        "legal_research": 1.0,
        "negotiation": 1.0,
        "default": 1.0,
    })

    def __post_init__(self):
        """Initialize with config-based overrides."""
        super().__post_init__()

        # Load from config if available
        if hasattr(self.config, '_raw_config') and 'payment' in self.config._raw_config:
            payment_cfg = self.config._raw_config['payment']
            self.base_fee_percent = payment_cfg.get('base_fee_percent', self.base_fee_percent)
            self.processing_time_seconds = payment_cfg.get('processing_time_seconds', self.processing_time_seconds)
            self.fraud_protection = payment_cfg.get('fraud_protection', self.fraud_protection)
            if 'supported_methods' in payment_cfg:
                self.supported_methods = payment_cfg['supported_methods']
            if 'rewards' in payment_cfg:
                for category, reward in payment_cfg['rewards'].items():
                    self.category_rewards[category] = reward

        logger.info(f"LegalPay initialized: {self.base_fee_percent}% base fee, 1% reward on all categories")
