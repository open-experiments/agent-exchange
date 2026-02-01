"""CompliancePay - Compliance & Regulatory payment specialist with advanced security."""

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
class CompliancePayAgent(BasePaymentAgent):
    """
    CompliancePay - Compliance & Regulatory Payment Specialist

    Characteristics:
    - Base fee: 3.0%
    - Rewards: UP TO 4% on compliance (NET CASHBACK!)
    - Net fee: -1.0% on compliance (you earn money!)
    - Processing: Thorough (8 seconds) - extra validation
    - Fraud protection: ADVANCED (AI-powered)

    Best for:
    - Compliance checks and audits
    - Regulatory filings
    - IP/Patent work
    - High-value transactions requiring extra security

    Fee breakdown:
    - Compliance: 3.0% - 4.0% = -1.0% (CASHBACK!)
    - IP/Patent: 3.0% - 3.5% = -0.5% (CASHBACK!)
    - Research: 3.0% - 2.0% = 1.0%
    - Contracts: 3.0% - 1.5% = 1.5%
    - Other: 3.0% - 1.0% = 2.0%
    """

    # Payment provider characteristics
    base_fee_percent: float = 3.0
    processing_time_seconds: int = 8
    supported_methods: list[str] = field(default_factory=lambda: ["card", "bank_transfer", "wire", "aex_balance"])
    fraud_protection: str = "advanced"

    # Category rewards - specializes in compliance
    category_rewards: dict[str, float] = field(default_factory=lambda: {
        "compliance": 4.0,          # CASHBACK territory!
        "compliance_check": 4.0,
        "ip_patent": 3.5,           # Also cashback!
        "regulatory": 4.0,
        "legal_research": 2.0,
        "contracts": 1.5,
        "contract_review": 1.5,
        "real_estate": 1.0,
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

        logger.info(f"CompliancePay initialized: {self.base_fee_percent}% base fee, UP TO 4% rewards on compliance!")
        logger.info(f"Advanced fraud protection enabled")
