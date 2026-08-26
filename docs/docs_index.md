# Documentation Index

This is the index of all documentation for StrataHub.

## Overview

- [README](README.md) - Strata platform overview and layered architecture philosophy
- [StrataHub Briefing](stratahub_briefing.md) - Platform briefing on orchestration, telemetry, and persistence

## Configuration

- [Configuration Guide](configuration.md) - Layered configuration system (config files, environment variables, CLI flags)

## Architecture

- [Multi-Tenancy and Roles](multi-tenancy-and-roles-architecture.md) - Workspaces, multi-role users, and coordinator role architecture
- [Naming and Vision](naming-and-vision.md) - Naming conventions and product direction
- [Time Progression](time_progression.md) - Academic cycles and time-based considerations

## Authentication

- [Auth Plan](auth/auth_plan.md) - Authentication architecture and planned features
- [Email Auth](auth/email_auth.md) - Email verification authentication (codes and magic links)
- [Email Auth Rate Limiting](auth/email_auth_rate_limiting.md) - Per-user rate limiting for email verification
- [Password Auth](auth/password_auth.md) - Password authentication and temporary password flow
- [Trust Auth](auth/trust_auth.md) - Zero-verification authentication for low-security scenarios

## UI/UX

- [Button Colors](button-colors.md) - Button color conventions throughout the application

## Database

- [Migration](migration.md) - Database migration scripts for MongoDB

## Mission HydroSci Integration

- [MHS Developer Sentinel User](mhs-dev-sentinel-user.md) - Well-known `stratahub.users` record seeded at startup so MHS editor/localhost play sessions round-trip cleanly through stratalog, stratasave, and mhsgrader

## Resource URL Identification

How StrataHub appends member/group/org/workspace identity to a resource's launch URL, configurable per resource.

- [ABT Survey URL Options](resource-identification/abt-survey-url-options.md) - Consumer-facing description of the two custom ABT schemes (identifiable vs de-identified) for the Abt survey links
- [Admin & Coordinator Guide](resource-identification/admin-coordinator-guide.md) - Choosing a URL identity scheme when creating/editing Resources
- [Data Recipient Guide](resource-identification/data-consumer-guide.md) - For consumers receiving the parameters: what each scheme sends and how it's encoded
- [Members Report — Resolving Identity from Hex IDs](resource-identification/members-report.md) - The authorized crosswalk that maps de-identified hex IDs back to names, orgs, groups, and logins
- [Parameter Vocabulary](resource-identification/vocabulary.md) - The permanent contract: definition of every parameter
- [Plan](resource-identification/plan.md) - Design and rollout of the identification modes

## Member Status API & Surveys Tab

An inbound, key-authenticated endpoint the survey provider calls when a student starts or completes a survey, and the MHS Dashboard tab that shows each student's survey status (not started / opened / started / completed).

- [Provider Guide](member-status-api/provider-guide.md) - The integration contract for the survey provider: endpoint, shared key, payload, survey names, semantics, error codes, examples
- [Admin Guide](member-status-api/admin-guide.md) - Setting the shared key, linking survey resources for the "Opened" state, reading the Surveys tab, changing the survey list, troubleshooting
- [Plan](member-status-api/plan.md) - Design, decisions, and task-by-task implementation record

## Deployment

- [Systemd Configuration](systemd_info.md) - Running StrataHub as a systemd service on Ubuntu Linux

## Logging & Analytics

- [Audit Logging Plan](audit_logging_plan.md) - Security event logging for admins and technical support
- [Activity Tracking Plan](activity_tracking_plan.md) - User activity tracking for teacher dashboards and research

## Roadmap & Strategy

- [Future Layers](future.md) - Planned future layers for the Strata platform
- [LMS Roadmap](lms_roadmap.md) - Path to evolving StrataHub into a Learning Management System
- [Research Platform Positioning](research_platform_positioning.md) - Strategic analysis for education research platform positioning
