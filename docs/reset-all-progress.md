# Reset All Progress — Mission HydroSci

## Overview

The "Reset All Progress" button in Mission HydroSci removes all downloaded unit files from the device and resets the client-side download state. It does not remove any data from servers.

The button is located in the **Manage downloads** section at the bottom of the Mission HydroSci units page.

> **Testers/Devs:** If you are logged in as a member, Reset All Progress requires a keyword. Get it from Discord or Jim Laffey.

## What It Does

1. Deletes all cached unit files from the device's storage
2. Clears local tracking data (manual download list)
3. Reloads the page to reflect the clean state

This is a client-side reset only. Progress data saved on servers (save.adroit.games, log.adroit.games) is not affected. Developers who need to clear server-side progress can do so directly via [save.adroit.games](https://save.adroit.games) and [log.adroit.games](https://log.adroit.games).

## Access by Role

### Non-member roles (admin, leader, coordinator, superadmin)

The button works with a simple confirmation dialog: "Reset all progress and remove all downloads?" This allows staff to quickly reset progress during development, testing, or troubleshooting.

### Members

Members are presented with a keyword prompt instead of a simple confirmation. Tapping "Reset All Progress" opens a modal with a **password field** (input is masked). The member — or more likely, the teacher standing at the member's device — must type the correct keyword to proceed. An incorrect keyword shows an error message and the reset does not execute.

The keyword is not published here. Developers and testers should get it from Discord or Jim Laffey.

The password field ensures that a teacher can type the keyword while the student is sitting in front of the screen without revealing it.

## Why the Keyword Gate Exists

Students should not casually reset their downloads. A full reset during the impact study forces re-downloading all unit content, which is disruptive on constrained school networks. The keyword gate prevents accidental or impulsive resets while still allowing authorized resets when needed.

## When Reset Is Useful

- **Testing**: Members of the development and QA team can reset a test account to replay the full experience without creating new accounts.
- **Troubleshooting**: If a download fails in a way that blocks progress — due to unreliable school networks, Chromebook storage issues, or unexpected edge cases — a teacher can reset the member's state and let them start fresh. This is a safety net for scenarios that are difficult to anticipate in resource-constrained school environments.
