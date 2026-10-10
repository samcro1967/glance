#!/usr/bin/env python3
"""Generate date-relative Calendar and ICS Events visual fixtures."""

from datetime import datetime, timedelta
from pathlib import Path


OUTPUT = Path(__file__).with_name("generated-calendar.ics")


def date_value(day):
    return day.strftime("%Y%m%d")


def utc_value(day, hour, minute=0):
    return day.strftime("%Y%m%d") + f"T{hour:02d}{minute:02d}00Z"


def floating_value(day, hour, minute=0):
    return day.strftime("%Y%m%d") + f"T{hour:02d}{minute:02d}00"


def event(uid, start, end, summary, *, location=None, url=None, rule=None):
    lines = [
        "BEGIN:VEVENT",
        f"UID:{uid}@glance",
        f"DTSTART{start}",
        f"DTEND{end}",
        f"SUMMARY:{summary}",
    ]
    if location:
        lines.append(f"LOCATION:{location}")
    if url:
        lines.append(f"URL:{url}")
    if rule:
        lines.append(f"RRULE:{rule}")
    lines.append("END:VEVENT")
    return lines


def generate(today):
    day = lambda offset: today + timedelta(days=offset)
    lines = [
        "BEGIN:VCALENDAR",
        "VERSION:2.0",
        "PRODID:-//Glance Visual QA//EN",
        "CALSCALE:GREGORIAN",
    ]

    lines += event(
        "visual-qa-1",
        ";VALUE=DATE:" + date_value(day(1)),
        ";VALUE=DATE:" + date_value(day(2)),
        "All-day design review",
        location="Design Studio",
        url="https://github.com/samcro1967/glance",
    )
    lines += event(
        "visual-qa-2",
        ":" + utc_value(day(2), 15),
        ":" + utc_value(day(2), 16, 30),
        "Glance release planning",
        location="Conference Room",
    )
    lines += event(
        "visual-qa-3",
        ":" + utc_value(day(4), 23, 30),
        ":" + utc_value(day(5), 1, 30),
        "Evening community event",
        location="St. Louis",
    )
    lines += event(
        "visual-qa-4",
        ";VALUE=DATE:" + date_value(day(6)),
        ";VALUE=DATE:" + date_value(day(9)),
        "Multi-day test event",
    )
    lines += event(
        "visual-qa-continuation",
        ":" + floating_value(day(9), 10),
        ":" + floating_value(day(10), 11),
        "Multi-day timed regression",
        location="Test Lab",
    )
    lines += event(
        "visual-qa-recurring",
        ":" + utc_value(day(0), 14),
        ":" + utc_value(day(0), 14, 30),
        "Weekly visual QA",
        rule="FREQ=WEEKLY;COUNT=6",
    )

    lines.append("END:VCALENDAR")
    return "\r\n".join(lines) + "\r\n"


def main():
    OUTPUT.write_bytes(generate(datetime.now().date()).encode("utf-8"))
    print(f"Generated visual calendar: {OUTPUT}")


if __name__ == "__main__":
    main()
