# Database Diagram — appointment-booking

```mermaid
erDiagram
    employee_service_config {
        string id_config PK
        string tenant_id
        string staff_id
        string service_id
        int duration_min
        int buffer_min
        float price_override
        bool payment_required
        bool is_active
    }

    reservation {
        string id_reservation PK
        string tenant_id
        string client_id
        string creator_user_id
        string employee_service_config_id
        string staff_idsnapshot
        string service_idsnapshot
        int duration_min_snapshot
        float price_snapshot
        string currency_snapshot
        int64 reservation_date
        int64 reservation_time
        string local_string_date
        string local_string_time
        string status
        string rescheduled_from_id
        string status_before_conflict
        string payment_id
        string notes
        int64 updated_at
        string updated_by
        int revision
    }

    workcalendar_config {
        string id_calendar_config PK
        string tenant_id
        string staff_id
        string timezone
        bool is_active
    }

    workcalendar_block {
        string id_block PK
        string tenant_id
        string staff_id
        int day_of_week
        int64 specific_date
        int start_min
        int end_min
        bool is_active
    }

    workcalendar_exception {
        string id_exception PK
        string tenant_id
        string staff_id
        int64 specific_date
        string exception_type
        int start_time
        int end_time
        string notes
    }

    workcalendar_config ||--o{ workcalendar_block : "staff-blocks"
    workcalendar_config ||--o{ workcalendar_exception : "staff-exceptions"
    employee_service_config ||--o{ reservation : "service-config"
```

> **Soft references (no physical FK):**
> - `reservation.client_id` → Directory/Clinical module
> - `reservation.creator_user_id` → IAM module
> - `employee_service_config.service_id` → Catalog module
> - `staff_id` fields → Staff module
> - `reservation.payment_id` → Payment module (nullable)
>
> **`workcalendar_config`** is the single source of truth for the IANA timezone of a staff member's calendar.
> `workcalendar_block` and `workcalendar_exception` do NOT carry timezone — they inherit it from `workcalendar_config`.
>
> **`workcalendar_block`** holds BOTH shapes in one table: `specific_date == 0` is a WEEKLY block
> (applies to `day_of_week`, `is_active` toggles it), `specific_date > 0` is a DATED block (applies
> to that date only and opens it even when the weekly template has nothing for that weekday). A day
> with several blocks ("morning + afternoon") is several rows; the lunch break is the GAP between
> them — there is no break column. `start_min`/`end_min` are minutes from midnight in the staff's
> local timezone.
>
> **`reservation.status`** is enforced by an in-code FSM (not a DB table). `CONFLICTED` is a
> non-terminal state whose `status_before_conflict` records where the reservation came from, so a
> recomputation can restore it exactly.
>
> **Irregular column names — do not "fix":** `staff_idsnapshot` / `service_idsnapshot` lack the
> underscore between `id` and `snapshot` (a historical snake_case quirk already live in production);
> a rename would force a destructive column rename in every deployed database.
>
> Availability rules (blocks + exceptions + the establishment's `time.DayBounds` window) are enforced
> at the service layer, not via DB relations.