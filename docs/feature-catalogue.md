---
sidebar_label: Feature catalogue
sidebar_position: 4
---

# Ember feature catalogue

**Wishlist snapshot: 20 November 2026.** This catalogue captures the desired
product direction from the supplied personal-Ember feature list. It is not a
release inventory and does not claim that these capabilities exist in this
repository.

The paths and access methods are retained as clues from the wishlist's
reference system. They are not paths in this repository, implementation
commitments, or endorsed technology choices. Ember should remain
provider-neutral and modular; see [modularity](architecture/modularity.md).

## 01 Agent experience and orchestration

| Wished-for capability | Intended outcome |
|---|---|
| Session rules, persona, skills, and hooks | A configurable, replaceable agent experience shaped by user-owned instructions and tools. |
| Session continuity and handoff | Long tasks can continue across bounded sessions without losing relevant context. |
| Large-file and image handling | Read only useful file ranges and appropriately sized images to reduce waste. |
| Search guidance and catch-up | Help users find relevant context and quickly resume ongoing work. |
| Parallel work lanes | Isolated, reviewable workspaces can handle independent tasks with controlled concurrency. |
| Remote sessions | Users can securely reach a session from another device. |
| Usage and cost visibility | Show activity and resource use so users can make informed choices. |
| On-demand connectors | Optional third-party connections are scoped, user-controlled, and not always enabled. |

## 02 Memory, knowledge, and search

| Wished-for capability | Intended outcome |
|---|---|
| User-owned knowledge vault | Portable Markdown notes, links, hubs, and metadata rather than opaque provider memory. |
| Daily notes and handoffs | Keep a useful record of current work and decisions over time. |
| Tasks, calendar, and roadmap | Make plans and open work available to the user and authorized integrations. |
| Hybrid search | Combine exact-text and semantic retrieval across authorized sources. |
| Local embedding option | Allow private, self-hosted indexing where hardware permits. |
| Import and distillation | Import conversations and help users turn useful material into durable notes. |
| Forget and export | Find and remove user data, and keep it exportable in documented formats. |
| Project notes | Keep project-specific context organized and connected to work records. |

## 03 Daily workflows and automation

| Wished-for capability | Intended outcome |
|---|---|
| Scheduled background work | Run user-approved routine tasks on a schedule with limits and clear reporting. |
| Daily review and briefing | Summarize activity, surface unfinished work, and propose next steps for user approval. |
| Notifications and failure alerts | Deliver useful updates while avoiding silent failures and unwanted interruptions. |
| Calendar views and feeds | Combine user calendars and authorized read-only feeds. |
| Quick capture and planning | Capture thoughts, plan priorities, and close out the day with low-friction workflows. |
| Research workflows | Research multiple sources and save results for review. |

## 04 Web workspace and integrations

| Wished-for capability | Intended outcome |
|---|---|
| Responsive workspace | Provide a coherent experience across desktop and mobile browsers. |
| Dashboard and knowledge views | Show system state, activity, history, and connected knowledge. |
| Chat and task streaming | Stream responses and progress, with uploads and the ability to stop work. |
| Approval cards | Make consequential actions visible and require policy checks or user approval. |
| Modular tools and MCP adapters | Expose explicitly authorized capabilities through replaceable integrations. |
| Browser integration | Allow user-approved interaction with browser tabs and content. |
| Terminal and media controls | Offer opt-in controls for user-selected devices and services. |
| Notes, statistics, and time tracking | Provide useful lightweight workspace tools without locking data in. |
| Video understanding | Summarize video using transcript and frame-based review. |

## 05 Voice

| Wished-for capability | Intended outcome |
|---|---|
| Voice activity detection and speech recognition | Support hands-free input, including configurable language and model choices. |
| Fast conversational response | Give a quick response path while longer work is delegated separately. |
| Worker coordination | Run bounded tasks with progress, pause, and cancellation controls. |
| Speech while working and text-to-speech | Speak progress and responses without obscuring user audio. |
| Acknowledgements and proactive alerts | Keep spoken updates concise and user-configurable. |
| Error handling and evaluation | Explain failures plainly and measure quality against consented test data. |

## 06 Desktop and device experiences

| Wished-for capability | Intended outcome |
|---|---|
| Thin desktop companion | A small native shell for workspace features that need desktop integration. |
| Desktop, taskbar, and app windows | Offer optional workspace surfaces without taking control away from the user. |
| Window and application controls | Perform explicitly requested actions through a permission-checked local adapter. |
| Context-aware panels | Show relevant system, media, or activity information with clear privacy controls. |
| Creative-tool connectors | Connect to applications such as Blender or FreeCAD through reviewed, replaceable adapters. |
| Self-update, crash reporting, and kill switch | Make updates understandable, failures diagnosable, and the integration easy to disable. |
| Native phone app | Provide native daily workflows while retaining a portable web experience. |

## 07 Design and fabrication studio

| Wished-for capability | Intended outcome |
|---|---|
| Project and artifact library | Browse real project files and outputs, with provenance and user control. |
| Live job progress | Show workflow stages and results as they happen. |
| CAD workflows | Create, inspect, edit, import, assemble, and document parametric parts. |
| Slicing and print preparation | Prepare print jobs and validate settings; starting a physical print requires explicit approval. |
| PCB workflows | Design, route, check, price, document, and import circuit boards. |
| Wiring and schematics | Create and review wiring plans and export printable documentation. |
| Rendering and 3D viewing | Generate and inspect stills, animations, parts, and assemblies. |
| Hand edits and external changes | Support user edits in native tools and reconcile changes safely. |

## 08 Skills and reusable workflows

Desired workflows include catch-up, handoff, capture, distillation, forgetting,
daily planning and close-out, logging, server checks, mail, video review,
rehearsal analysis, test review, CAD, PCB, wiring, rendering, and creative-tool
control. These should be portable recipes or modules—not assumptions that one
agent, provider, or skill format is mandatory.

## 09 Server and operations

| Wished-for capability | Intended outcome |
|---|---|
| Health checks and monitoring | Check services and host health with actionable thresholds and alerts. |
| Backups and restore testing | Back up user data and regularly verify that recovery works. |
| Network and access controls | Keep private services private and make remote access deliberate. |
| Update and cleanup policies | Handle maintenance predictably and keep temporary work bounded. |
| Sandboxed task execution | Isolate untrusted or risky work, audit access, and clean up after execution. |
| Optional GPU services | Make local speech, vision, or rendering services optional deployment components. |
| Operational logbook | Keep an auditable record of changes and important events. |

## 10 Quality and testing

| Wished-for capability | Intended outcome |
|---|---|
| Unit and integration tests | Verify behavior at module and contract boundaries. |
| Visual checks | Catch interface regressions with reproducible screenshots and review. |
| Isolated try-out environments | Test changes without affecting a user's live workspace or data. |
| Independent review | Review consequential changes and record findings. |
| Evaluation suites | Measure search, voice, design, and agent quality against suitable test data. |
| User test review | Help users interpret their own test runs from evidence, not unsupported claims. |

## 11 Later possibilities and dependencies

These are longer-horizon ideas, not commitments or shipped features:

- In-game voice and context-aware assistance.
- A voice-controlled globe for maps, travel, weather, or transport.
- Home automation through an explicit integration.
- Sending prints and monitoring printer status, gated by user approval and hardware.
- Quieter or more capable server hardware and off-site backup.
- A separate local-AI machine, if budget and operational needs justify it.
- Separate household profiles with isolated memory, settings, and permissions.
- Searchable photo and paper-document libraries.
- Broader automated review when usage budgets and safeguards are ready.

## Current implementation

This wishlist is broader than the software in this repository. Today, the
runtime is a Go health API and static Nginx frontend; agent sessions, voice,
memory, fabrication workflows, authentication, persistence, and the other
catalogued capabilities are not implemented. See [architecture status](architecture/README.md)
and [core installation](core-installation.md).
