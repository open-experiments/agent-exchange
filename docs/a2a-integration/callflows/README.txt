AEX ↔ A2A PlantUML Diagrams

Files
- 01_provider_onboarding_sequence.puml
- 02_work_bidding_award_sequence.puml
- 03_execution_settlement_sequence.puml
- 04_architecture_component.puml
- 05_end_to_end_activity.puml

Render (examples)
1) PlantUML Server:
   plantuml -tpng 01_provider_onboarding_sequence.puml

2) Docker:
   docker run --rm -v "$PWD":/work -w /work plantuml/plantuml -tpng 01_provider_onboarding_sequence.puml
