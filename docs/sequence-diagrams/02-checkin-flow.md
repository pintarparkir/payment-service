# Check-in Flow - Sequence Diagram

> **Purpose:** Detailed flow for driver check-in with geofence validation  
> **Related Docs:** [`reservation-service/README.md`](../../../reservation-service/README.md) · [`docs/architecture/erd/02-reservation-service.md`](../erd/02-reservation-service.md)  
> **Author:** Solution Architecture · **Last Updated:** 2026-06-01

---

## 📖 Overview

Flow ini mencakup:
1. **Geofence validation** — Haversine distance calculation between driver's GPS and building location
2. **State transition** — Reservation transitions from CONFIRMED → ACTIVE
3. **Error handling** — User outside geofence, invalid state, expired hold

### Participants

| Name | Type | Responsibility |
|------|------|----------------|
| 👤 Driver | Actor | Initiates check-in from building vicinity |
| 📱 Mini-App | Client | Captures GPS coordinates, displays result |
| 🔌 Reservation Service | gRPC | State machine transition & geofence validation |
| 💾 Postgres DB | Relational | Reservation state persistence |

---

## 🔢 Sequence Diagram

```mermaid
sequenceDiagram
    autonumber
    actor 👤 as 👤 Driver
    participant 📱 as 📱 Mini-App
    participant 💾 as 💾 Reservation Service
    participant 💾_DB as 💾 Postgres DB

    Note over 👤,💾_DB: ──────────────────────────────────────────────
    PHASE 1: Driver Initiates Check-in
    Note over 👤,💾_DB: ──────────────────────────────────────────────
    
    👤->>📱: Tap "Check-In" button
    Note right of 📱: App automatically requests GPS permission<br/>or uses last known location
    
    📱->>📱: Acquire GPS coordinates
    Note right of 📱: lat: -6.20015<br/>lon: 106.81705<br/>accuracy: 12m
    
    alt GPS unavailable or permission denied
        📱->>📱: Show manual location input screen
        👤->>📱: Enter building name or code (fallback)
    end
    
    📱->>💾: POST /v1/reservations/{id}/check-in<br/>{lat: -6.20015, lon: 106.81705}
    activate 💾
    
    %% Step 1: Fetch reservation with lock
    💾->>💾_DB: BEGIN TRANSACTION
    
    %% SELECT FOR UPDATE prevents race with concurrent operations
    💾->>💾_DB: SELECT * FROM reservation<br/>WHERE id = ? FOR UPDATE
    activate 💾_DB
    
    % Reservation not found
    alt Reservation not found
        💾_DB-->>💾: 0 rows
        💾->>💾_DB: COMMIT
        💾-->>📱: 404 Not Found "Reservation does not exist"
        deactivate 💾
        return
    end
    
    💾_DB-->>💾: Row<br/>{state: 'CONFIRMED',<br/>checked_in_at: nil,<br/>hold_end: 2026-06-01T09:10:00Z,<br/>driver_id: uuid}
    deactivate 💾_DB
    
    Note over 👤,💾_DB: ──────────────────────────────────────────────
    PHASE 2: Validate Prerequisites
    Note over 👤,💾_DB: ──────────────────────────────────────────────
    
    %% Step 2: State validation
    alt state != 'CONFIRMED'
        alt state == 'ACTIVE'
            💾-->>📱: 400 Bad Request "Already checked in"
        else state == 'COMPLETED'
            💾-->>📱: 400 Bad Request "Reservation already completed"
        else state == 'CANCELLED'
            💾-->>📱: 400 Bad Request "Reservation was cancelled"
        else state == 'EXPIRED'
            💾-->>📱: 400 Bad Request "Reservation expired (no-show)"
        else state == 'PENDING'
            💾-->>📱: 400 Bad Request "Please confirm reservation first"
        else
            💾-->>📱: 400 Bad Request "Invalid state for check-in"
        end
        💾->>💾_DB: COMMIT
        deactivate 💾
        return
    end
    
    %% Step 3: Hold time validation
    💾->>💾_DB: SELECT NOW() AS current_time
    activate 💾_DB
    💾_DB-->>💾: current_time = '2026-06-01T09:05:00Z'
    deactivate 💾_DB
    
    alt NOW() > hold_end
        %% Hold expired
        💾-->>📱: 410 Gone "Reservation hold expired. Create a new reservation."
        💾->>💾_DB: COMMIT
        deactivate 💾
        return
    end
    
    Note over 👤,💾_DB: ──────────────────────────────────────────────
    PHASE 3: Geofence Validation
    Note over 👤,💾_DB: ──────────────────────────────────────────────
    
    %% Step 4: Distance calculation via Haversine
    💾->>💾: HaversineDistanceCalculation(
        lat1: -6.20015, lon1: 106.81705,
        lat2: -6.20880, lon2: 106.84560  <!-- Building coordinates -->
    )
    
    Note right of 💾: ┌─────────────────────────────────────────┐
                        │ Haversine formula:                     │
                        │                                        │
                        │ a = sin²(Δlat/2) + cos(lat1) ⋅        │
                        │     cos(lat2) ⋅ sin²(Δlon/2)          │
                        │                                        │
                        │ c = 2 ⋅ atan2(√a, √(1-a))             │
                        │                                        │
                        │ d = R ⋅ c   (R = 6,371 km)            │
                        │                                        │
                        │ Implementation: stdlib-only ~50 LOC    │
                        └─────────────────────────────────────────┘
    
    💾->>💾: distance = 187 meters
    Note right of 💾: Building Threshold: GEOFENCE_RADIUS_METERS = 150m
    
    alt distance <= 150m (configurable threshold)
        %% ✅ Inside geofence
        Note over 💾,💾_DB: ──────────────────────────────────────────
        PHASE 4: State Transition → ACTIVE
        Note over 💾,💾_DB: ──────────────────────────────────────────
        
        %% Step 5: Update state to ACTIVE
        💾->>💾_DB: UPDATE reservation SET <br/>state = 'ACTIVE',<br/>checked_in_at = NOW(),<br/>driver_lat = -6.20015,<br/>driver_lon = 106.81705<br/>WHERE id = ?
        activate 💾_DB
        💾_DB-->>💾: 1 row affected
        deactivate 💾_DB
        
        %% Step 6: Record audit trail (optional)
        💾->>💾_DB: INSERT INTO reservation_audit (<br/>reservation_id, event_type='CHECK_IN',<br/>metadata: {lat, lon, distance_in_meters: 187})
        
        💾->>💾_DB: COMMIT
        
        %% ✅ Success response
        💾-->>📱: 200 OK {<br/>"state": "ACTIVE",<br/>"checked_in_at": "2026-06-01T09:05:00Z",<br/>"spot_id": "F2-C-014",<br/>"message": "Welcome! Enjoy your parking."<br/>}
        
        deactivate 💾
        
        %% Background: Optional SMS notification
        opt Send Welcome SMS (background)
            💾->>RabbitMQ: PUBLISH reservation.checked_in.v1
            Note right of 💾: For notification-service<br/>Template: "Selamat datang!<br/>Check-in berhasil."
        end
        
    else distance > 150m
        %% ❌ Outside geofence
        💾-->>📱: 409 Conflict {<br/>"code": "OUTSIDE_GEOFENCE",<br/>"message": "You are {187}m away from the building.",<br/>"distance_meters": 187,<br/>"geofence_radius_meters": 150,<br/>"building_location": {-6.20880, 106.84560}<br/>}
        
        💾->>💾_DB: ROLLBACK
        
        📱->>📱: Show map with:<br/>- User's location (red pin)<br/>- Building location (blue pin)<br/>- Geofence radius (dashed circle)
        
        deactivate 💾
        return
    end
    
    Note over 👤,💾_DB: ──────────────────────────────────────────────
    Summary: ✅ Check-in successful
    Note over 👤,💾_DB: Total distance: 187m | Threshold: 150m
    Note over 👤,💾_DB: State transition: CONFIRMED → ACTIVE
    Note over 👤,💾_DB: ──────────────────────────────────────────────
```

---

## 🧮 Haversine Formula Implementation

```go
// pkg/geofence/geofence.go

const (
    EarthRadiusKm = 6371.0
    DegreesToRad  = math.Pi / 180.0
)

type Coordinates struct {
    Latitude  float64
    Longitude float64
}

func HaversineDistance(p1, p2 Coordinates) float64 {
    dLat := (p2.Latitude - p1.Latitude) * DegreesToRad
    dLon := (p2.Longitude - p1.Longitude) * DegreesToRad
    
    a := math.Sin(dLat/2)*math.Sin(dLat/2) +
        math.Cos(p1.Latitude*DegreesToRad)*math.Cos(p2.Latitude*DegreesToRad)*
        math.Sin(dLon/2)*math.Sin(dLon/2)
    
    c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
    
    return EarthRadiusKm * c // Returns distance in kilometers
}

func IsWithinGeofence(user, building Coordinates, radiusMeters float64) bool {
    distanceKm := HaversineDistance(user, building)
    return distanceKm*1000 <= radiusMeters
}
```

**Test Cases:**

| Input (distance) | Threshold | Expected |
|------------------|-----------|----------|
| 45m | 150m | ✅ Inside |
| 150m | 150m | ✅ Inside (exactly at boundary) |
| 187m | 150m | ❌ Outside |
| -6.2000, 106.8166 | 150m | ✅ Inside (exact building location) |

---

## 🐛 Error Scenarios & Handling

| Scenario | HTTP Status | Response Data | User Action |
|----------|-------------|---------------|-------------|
| **GPS unavailable** | — | — | Show manual entry or building code |
| **Reservation not found** | 404 | `{error: "reservation not found"}` | Create new reservation |
| **Wrong state (PENDING)** | 400 | `{error: "confirm reservation first"}` | Confirm spot first |
| **Already checked in** | 400 | `{error: "already checked in at {time}"}` | Continue parking |
| **Reservation expired** | 410 | `{error: "hold expired at {time}"}` | Create new reservation |
| **Reservation cancelled** | 400 | `{error: "reservation was cancelled"}` | Create new reservation |
| **Outside geofence** | 409 | `{distance, threshold, building_loc}` | Move closer to building |
| **Building coordinates not configured** | 500 | `{error: "geofence not configured"}` | Contact support |
| **Concurrent check-in** | 409 | `{error: "being processed by another request"}` | Retry |

---

## 🔄 Related Flows

| Previous Flow | This Flow | Next Flow |
|---------------|-----------|-----------|
| Reserve & Confirm (01-reservation-flow) | **Check-in (Geofence)** | Check-out & Billing (03-billing-checkout-flow) |

### State Machine Transition

```
PENDING ──confirm──▶ CONFIRMED ──check-in──▶ ACTIVE ──check-out──▶ COMPLETED
```

---


