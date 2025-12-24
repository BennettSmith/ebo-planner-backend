# Overland Trip Planning – v1 Domain Model & Use Case Catalog

## Purpose

This document defines the **v1 domain model** and **use case catalog** for a members-only system that coordinates the **planning** of overlanding trips.

This system exists to replace the previously used “Rally Point” system and is intentionally **not** a social platform. Its sole purpose is to support trip planning and RSVP coordination.

Execution of trips happens offline (radio, maps, in person).

---

## Core Principles (v1 Guardrails)

- Planning only (no live execution or tracking)
- Members-only system (no unauthenticated users)
- RSVP is lightweight and member-owned
- Capacity is measured in **vehicles (rigs)**, not people
- Transparency: all members can see trip rosters
- No social features (no feeds, comments, chat, likes)
- Artifacts are referenced externally (not hosted)

---

## Domain Model

### Aggregates

#### Trip (Aggregate Root)

A Trip represents a coordinated plan for an overlanding trip.

##### Lifecycle States
- `DRAFT`
- `PUBLISHED`
- `CANCELED`

##### Draft Visibility
(Only relevant when `status = DRAFT`)
- `PRIVATE` – visible only to organizers
- `PUBLIC` – visible to all members (no RSVP)

##### Required at Publish Time
A trip **may be incomplete** while in draft. The following fields are required **only when publishing**:

- name
- description
- start date
- end date
- capacity (number of rigs)
- difficulty (free-form text)
- meeting location
- communications requirements
- recommended requirements
- at least one organizer

##### Fields (v1)
- `TripId`
- `name`
- `description`
- `startDate`
- `endDate`
- `status` (`DRAFT | PUBLISHED | CANCELED`)
- `draftVisibility` (`PRIVATE | PUBLIC`)
- `capacityRigs` (integer, hard limit)
- `difficultyText` (free-form)
- `meetingLocation` (Location)
- `commsRequirementsText` (free-form)
- `recommendedRequirementsText` (free-form)
- `organizerMemberIds[]`
- `artifacts[]` (TripArtifact)

##### Invariants
- RSVP is only allowed when `status = PUBLISHED`
- Capacity is enforced strictly on RSVP = YES
- Organizers may edit any field at any time
- A trip may be canceled at any time by an organizer
- At least one organizer must always exist

---

#### RSVP (Entity within Trip)

Represents a member’s intent to attend a trip with **one vehicle**.

##### Fields
- `TripId`
- `MemberId`
- `response` (`YES | NO | UNSET`)
- `updatedAt`

##### Rules
- Owned by the member
- Member may change RSVP at any time
- `YES` consumes one rig slot
- `NO` and `UNSET` do not consume capacity
- Block `YES` when capacity is reached
- No waitlist in v1

---

#### Member

Authenticated account (identity provided by Auth Genie or equivalent).

##### Fields (v1)
- `MemberId`
- `displayName`
- `email`
- `groupAliasEmail?`
- `vehicleProfile?`

##### Rules
- Any authenticated member may RSVP
- All members can view trips and rosters
- All members can search the member directory

---

#### VehicleProfile (Value Object on Member)

Represents a member’s **primary vehicle** (v1 assumption).

##### Fields
- make / model
- tire size
- lift / lockers
- fuel range
- recovery gear
- ham radio call sign
- notes

> Vehicle capability is **informational only** and not enforced.

---

### Value Objects

#### Location
- `label`
- `address?`
- `latitudeLongitude?`

Supports either a physical address or lat/lon (or both).

#### TripArtifact
- `type` (GPX, schedule, document, other)
- `title`
- `url` (external storage such as Google Drive or S3)

---

## Use Case Catalog (v1)

### Trip Discovery

#### 1. ListVisibleTripsForMember
**Actor:** Member  
**Description:**  
Returns all trips visible to a member:
- Published trips
- Public drafts

---

#### 2. GetTripDetails
**Actor:** Member  
**Description:**  
Retrieve full trip details, including logistics, artifacts, and RSVP summary.

---

### Trip Planning & Administration

#### 3. CreateTripDraft
**Actor:** Organizer  
**Description:**  
Create a new trip in `DRAFT` state. Incomplete data is allowed.

---

#### 4. UpdateTripDraft
**Actor:** Organizer  
**Description:**  
Update any fields on a draft trip. Partial updates allowed.

---

#### 5. SetTripDraftVisibility
**Actor:** Organizer  
**Description:**  
Toggle a draft between private and public visibility.

---

#### 6. PublishTrip
**Actor:** Organizer  
**Description:**  
Publish a trip, enforcing required fields and sending a **single announcement email** to the Google Group mailing list.

---

#### 7. UpdatePublishedTrip
**Actor:** Organizer  
**Description:**  
Edit any aspect of a published trip (logistics may evolve).

---

#### 8. CancelTrip
**Actor:** Organizer  
**Description:**  
Cancel a trip. RSVPs are disabled and the trip becomes read-only.

---

### Organizer Management

#### 9. AddTripOrganizer
**Actor:** Organizer  
**Description:**  
Add an existing member as a co-organizer for a trip.

---

#### 10. RemoveTripOrganizer
**Actor:** Organizer  
**Description:**  
Remove an organizer from a trip. At least one organizer must remain.

---

### RSVP Coordination

#### 11. SetMyRSVP
**Actor:** Member  
**Description:**  
Set RSVP to `YES`, `NO`, or `UNSET` for a published trip.

---

#### 12. GetTripRSVPSummary
**Actor:** Member  
**Description:**  
View lists of attending and not-attending members.

---

#### 13. GetMyRSVPForTrip
**Actor:** Member  
**Description:**  
Retrieve the member’s current RSVP state for a trip.

---

### Member Directory

#### 14. ListMembers
**Actor:** Member  
**Description:**  
View a minimal directory of members for coordination.

---

#### 15. SearchMembers
**Actor:** Member  
**Description:**  
Search members by name or email (used for adding organizers).

---

### Member Profile

#### 16. UpdateMyMemberProfile
**Actor:** Member  
**Description:**  
Update profile metadata including group alias and vehicle profile.

---

## Explicit Non-Goals (v1)

- No social feeds, comments, or messaging
- No live tracking or execution support
- No payments or monetization
- No heavy mapping UI
- No enforcement of vehicle capability
- No waitlists (planned v2)

---

## Forward Compatibility Notes

v1 is intentionally simple but designed to evolve toward:
- Waitlists
- Multiple vehicles per member
- Passenger counts (adult/child)
- Structured difficulty ratings
- Artifact hosting integrations

These are **explicitly deferred** beyond v1.