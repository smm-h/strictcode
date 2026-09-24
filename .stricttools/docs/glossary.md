+++
title = "Glossary"
description = "The terms strictcode's documentation uses, from interaction relation and projection to capability, profile, lesson, and suppression shape."
nav_order = 440
+++

# Glossary

:<: list-glossary
:=:
::: **Interaction relation**: The flat table of typed rows, each linking a source node to a destination node at a byte span, that is strictcode's source of truth.
::: **Node table**: The companion table of nodes, each with a kind, a qualified ID, and attributes.
::: **Projection**: A deterministic view derived from the interaction relation, such as the algorithm graph or the site feed.
::: **Side table**: Extraction data kept outside the relation because a site has no node at one end, such as external imports and external or unresolved calls.
::: **Qualified ID**: A node's public identity, built from its language, workspace member, logical module name, and container chain.
::: **Capability**: A fine-grained unit of extraction, such as resolving imports to workspace members, that profiles declare and rules require.
::: **Layer**: A named bundle of capabilities used to present the support matrix, such as the import graph or the full semantic graph.
::: **Profile**: A language's declaration of how its constructs map onto the vocabulary and the status of every capability.
::: **Maturity**: Whether a vocabulary kind is stable, meaning exercised by an extractor or rule, or speculative, meaning designed ahead of use.
::: **Requires and uses**: A rule's two capability lists: required capabilities decide support, and used capabilities only enrich the diagnosis.
::: **Lesson**: A numbered regression requirement from the lessons register, each implemented as a red-green test.
::: **Suppression shape**: The form a rule's suppressions take: a path, a member and dependency pair, a cycle's module set, a member, or none.
::: **Retirement record**: The registry entry kept for a retired rule, saying when and why it was retired, what replaced it, and what to do.
::: **Group**: A named switch over several rules, written with a group prefix, that configuration can toggle in one entry.
::: **Fix tier**: The safety class of a fix: guaranteed behavior-preserving, behavior-changing with consent, or suggestion only.
::: **Declared delta**: The rows and nodes a tier-1 fix is expected to remove, against which the re-extracted graph is verified.
:>:
