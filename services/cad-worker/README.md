# CAD worker (planned)

This component is not implemented. It will execute a constrained CAD recipe
and return editable source plus validated STEP/STL artifacts.

The worker interface should declare tool/version, input files, resource
requirements, allowed network policy, output manifest, and validation results.
It must not access fabrication devices; artifact generation and fabrication
authorization are separate workflows.
