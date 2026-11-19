---
sidebar_label: Inspiration
sidebar_position: 3
---

# Inspiration: projects and products to learn from

Ember is a design and fabrication workspace, not a clone of any one assistant,
CAD application, or printer dashboard. These projects and products are
references for particular capabilities and integration patterns. Their
inclusion does not imply endorsement, compatibility, or a commitment to depend
on them.

## Design and creation tools

| Project/product | What to learn from it | Potential Ember connection |
| --- | --- | --- |
| [FreeCAD](https://www.freecad.org/) | Parametric 3D CAD and editable, engineering-oriented source models. | Open, inspect, revise, and validate mechanical designs within a project workflow. |
| [KiCad](https://www.kicad.org/) | Integrated schematic capture and PCB layout, with manufacturing outputs. | Keep electronics design sources, checks, and fabrication exports together. |
| [Blender](https://www.blender.org/) | General 3D modeling, rendering, and a mature scripting interface. | Connect visual design, previews, and generated assets to project revisions. |
| [OpenSCAD](https://openscad.org/) | Scriptable, reproducible solid modeling. | Generate and version parametric designs from editable source. |

## Fabrication and machine control

| Project/product | What to learn from it | Potential Ember connection |
| --- | --- | --- |
| [OctoPrint](https://octoprint.org/) | 3D-printer operation, job monitoring, and a plugin-based integration model. | A candidate adapter for printer status and explicitly approved print jobs. |
| [Klipper](https://www.klipper3d.org/) and [Moonraker](https://moonraker.readthedocs.io/) | Firmware and API patterns for controlling and monitoring 3D printers. | A candidate path to printer readiness, job progress, and machine telemetry. |
| [CNCjs](https://cnc.js.org/) | Browser-based CNC control and machine visualization. | Reference for presenting CNC state and supervised machine operations. |
| [LinuxCNC](https://linuxcnc.org/) | Real-time CNC machine control and an extensible control environment. | Potential integration target where a shop uses LinuxCNC; machine safety remains authoritative. |

## Connected workshop and physical interfaces

| Project/product | What to learn from it | Potential Ember connection |
| --- | --- | --- |
| [Home Assistant](https://www.home-assistant.io/) | Broad device integration, dashboards, and local automation. | Optional bridge for workshop sensors, lights, power monitoring, and device state—not the design workspace itself. |
| [MQTT](https://mqtt.org/) | Lightweight messaging used by many embedded and workshop devices. | A transport for status and commands where device adapters and authorization are carefully scoped. |
| [Touchscreen / wall-tablet dashboards](https://www.home-assistant.io/dashboards/) | At-a-glance information and touch-friendly control on a dedicated display. | Inspiration for a desk-mounted studio screen showing project plans, design previews, machine status, and approval prompts. |

## AI workspaces and engineering agents

| Project | What to learn from it | Potential Ember connection |
| --- | --- | --- |
| [Odysseus](https://github.com/odysseus-dev/odysseus) | A broad self-hosted AI workspace with chat, agents, research, documents, and personal tools. | Reference for workspace breadth and self-hosted deployment; not a substitute for fabrication integrations. |
| [OpenHands](https://github.com/All-Hands-AI/OpenHands) | Engineering agents and ways to run them against different backends. | A candidate reference or replaceable runner for software tasks related to a making project. |
| [OpenSandbox](https://github.com/opensandbox-group/opensandbox) | Isolated execution environments and sandbox lifecycle APIs for agents. | A candidate building block for safely running untrusted design scripts or engineering tasks. |

## Design principles for Ember

- Organize the experience around **projects and artifacts**, not a generic chat transcript.
- Preserve editable source files, generated outputs, tool versions, machine settings, and provenance.
- Make previews and validation visible before an artifact or job reaches equipment.
- Treat physical-device commands as consequential actions: authorize them in host services and require approval where appropriate.
- Use explicit adapters so CAD tools, printers, CNC machines, and displays can be replaced independently.
- Do not assume that local hosting makes model-provider requests local, or that a connected machine is safe to operate automatically.

This list is a starting point for product discovery. Before adopting a project as a
dependency or integration, evaluate its current APIs, maintenance, license,
security model, and hardware compatibility under the
[library and implementation policy](decisions/004-go-and-library-policy.md).
