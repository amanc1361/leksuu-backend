"""Deterministic policy for automatic preparation; never grants media rights.

Automatic acceptance is explicitly labelled and is not a human review.
Keep this module independent of inference and database libraries for testing.
"""
import copy
import difflib
import hashlib
import json
import math
import re

VERSION = 2
ACCEPTED = {"approved", "auto_approved"}


def tokens(value):
    return re.findall(r"[^\W_]+", value.casefold(), re.UNICODE)


def identity(payload):
    return {"url": payload.get("url", ""), "audio_sha256": payload.get("audio_sha256", ""),
            "language": payload.get("language", "en"),
            "transcript_texts": [cue.get("text", "") for cue in (payload.get("transcript") or [])]}


def translation_hash(payload):
    # Match encoding/json's struct field order and HTML escaping in the API.
    source = {key: payload.get(key, "") for key in ("language", "source_id", "url", "audio_sha256")}
    source["transcript"] = [{"text": cue.get("text", "")} for cue in (payload.get("transcript") or [])]
    raw = json.dumps(source, ensure_ascii=False, separators=(",", ":"))
    for character in ("<", ">", "&", "\u2028", "\u2029"):
        raw = raw.replace(character, "\\u%04x" % ord(character))
    return hashlib.sha256(raw.encode()).hexdigest()


def translation_input(payload, language):
    return [{"cue_id": cue.get("cue_id"), "text": cue.get("text", ""),
             "translation": cue.get("translation", {}).get(language, "")}
            for cue in (payload.get("transcript") or [])]


def media_source(payload):
    return {key: payload.get(key, "") for key in ("url", "audio_sha256", "media_sha256")}


def source_input(payload):
    return [{key: cue.get(key) for key in ("cue_id", "text", "start", "end")}
            for cue in (payload.get("transcript") or [])]


def next_stage(kind, payload):
    """Serial stages avoid invalidating another job's payload snapshot."""
    if kind not in ("video", "music", "podcast", "story") or payload.get("published") is not False or not payload.get("url"):
        return None
    if kind in ("video", "music") and (not payload.get("hls_manifest_key") or payload.get("hls_source") != media_source(payload)):
        return "transcode", ""
    report = payload.get("automation", {}).get("source") or {}
    if payload.get("alignment_source") != identity(payload) or report.get("input") != source_input(payload) or (report.get("passed") and any(
            not cue.get("editorial_review") or not cue.get("timing_status") for cue in (payload.get("transcript") or []))):
        return "synchronize", ""
    cues = payload.get("transcript") or []
    if not cues:
        return None
    for language in ("fa", "ckb"):
        if all(cue.get("translation", {}).get(language) and cue.get("translation_review", {}).get(language) in ACCEPTED for cue in cues):
            continue
        report = payload.get("automation", {}).get("translations", {}).get(language)
        if not report or report.get("input") != translation_input(payload, language):
            return "translate", language
    return None


def source_assessment(cues, witness, duration):
    """Compare with a second recognizer, rather than self-validating ASR text."""
    observed = [(token, word["start"], word["end"], word.get("probability", 0))
                for segment in witness.get("segments", []) for word in segment.get("words", [])
                for token in tokens(word.get("word", ""))]
    expected = [(token, i) for i, cue in enumerate(cues) for token in tokens(cue.get("text", ""))]
    matcher = difflib.SequenceMatcher(None, [t[0] for t in expected], [t[0] for t in observed], autojunk=False)
    matches = {block.a + j: block.b + j for block in matcher.get_matching_blocks() for j in range(block.size)}
    issues = []
    previous = 0
    for i, cue in enumerate(cues):
        reasons = []
        start, end = cue.get("start"), cue.get("end")
        if not all(isinstance(v, (int, float)) and math.isfinite(v) for v in (start, end)) or not previous <= start < end <= duration + .3:
            reasons.append("invalid_timing")
        else:
            previous = end
        positions = [j for j, (_, owner) in enumerate(expected) if owner == i]
        pairs = [observed[matches[j]] for j in positions if j in matches]
        if not positions or len(pairs) != len(positions):
            reasons.append("recognizers_disagree_on_words")
        if pairs:
            if min(word[3] for word in pairs) < .85:
                reasons.append("low_recognition_confidence")
            if "invalid_timing" not in reasons and (abs(pairs[0][1] - start) > .75 or abs(pairs[-1][2] - end) > .75):
                reasons.append("recognizers_disagree_on_timing")
        if reasons:
            issues.append({"paragraph": i + 1, "reasons": reasons})
    used = set(matches.values())
    missing = [word for i, word in enumerate(observed) if i not in used]
    if missing:
        nearest = min(range(len(cues)), key=lambda i: abs((cues[i].get("start") or 0) - missing[0][1])) if cues else 0
        issues.append({"paragraph": nearest + 1, "reasons": ["possible_missing_words"],
                       "candidate_start": missing[0][1], "candidate_end": missing[-1][2]})
    return {"version": VERSION, "issues": issues, "passed": bool(cues) and not issues,
            "verification_model": witness.get("model", ""), "duration_seconds": duration}


def translation_assessment(source, translation, backtranslation, independent=False, target=None):
    original, restored = tokens(source), tokens(backtranslation)
    score = difflib.SequenceMatcher(None, original, restored, autojunk=False).ratio()
    letters = [c for c in translation if c.isalpha()]
    script = sum("\u0600" <= c <= "\u08ff" for c in letters) / max(1, len(letters))
    from translation_quality import candidate_risks
    risks = candidate_risks(source, translation, target, backtranslation) if target else []
    reason = ("independent_verification_unavailable" if not independent else
              "wrong_script" if script < .7 else
              risks[0] if risks else
              "roundtrip_mismatch" if score < .85 or not original or not restored else "")
    return {"passed": not reason,
            "roundtrip_similarity": round(score, 4), "method": "independent_roundtrip_and_script_v2",
            "independent_model": bool(independent), "reason": reason, "risks": risks}


def automatic_publication_check(kind, payload):
    """Fail closed on stale evidence, uncertain cues, rights, and missing opt-in."""
    issues = []
    if kind not in ("video", "music", "podcast") or payload.get("published") is not False:
        issues.append("unsupported_or_published")
    if payload.get("auto_publish") is not True:
        issues.append("owner_opt_in_required")
    if payload.get("rights_approved") is not True or not str(payload.get("rights_reference") or "").strip():
        issues.append("rights_required")
    if kind in ("video", "music") and (not str(payload.get("hls_manifest_key") or "").startswith("hls/") or
                                       payload.get("hls_source") != media_source(payload)):
        issues.append("hls_required")
    report = payload.get("automation") or {}
    source = report.get("source") or {}
    if (not source.get("passed") or source.get("issues") or not source.get("verification_model") or
            source.get("input") != source_input(payload) or payload.get("alignment_source") != identity(payload)):
        issues.append("source_verification_required")
    # A second ordinary speech recognizer is not independent evidence that
    # vocals were recovered through instruments or repeated choruses.
    if kind == "music" and source.get("music_vocal_verification") != "passed":
        issues.append("music_vocal_verification_required")
    cues = payload.get("transcript") or []
    if not cues:
        issues.append("source_cues_required")
    duration = payload.get("media_duration_seconds")
    if not isinstance(duration, (int, float)) or not math.isfinite(duration) or duration <= 0:
        issues.append("verified_duration_required")
    previous = 0
    seen = set()
    for cue in cues:
        start, end = cue.get("start"), cue.get("end")
        cue_id = cue.get("cue_id")
        if not cue_id or cue_id in seen:
            issues.append("stable_cue_ids_required")
        seen.add(cue_id)
        if (not all(isinstance(value, (int, float)) and math.isfinite(value) for value in (start, end)) or
                not previous <= start < end or
                (isinstance(duration, (int, float)) and math.isfinite(duration) and end > duration + .3)):
            issues.append("cue_timing_required")
        else:
            previous = end
        if not tokens(cue.get("text", "")) or cue.get("timing_status") != "auto_approved" or \
                (cue.get("editorial_review") or {}).get("decision") != "auto_approved":
            issues.append("cue_source_approval_required")
        if kind != "podcast" and cue.get("exercise_review") != "auto_approved":
            issues.append("exercise_approval_required")
        translations = cue.get("translation") or {}
        if translations.get("fa") and translations.get("ckb") and \
                translations["fa"].strip() == translations["ckb"].strip():
            issues.append("language_substitution_suspected")
    for language in ("fa", "ckb"):
        check = (report.get("translations") or {}).get(language) or {}
        if (not check.get("passed") or check.get("issues") or
                not (check.get("calibration") or {}).get("passed") or
                check.get("input") != translation_input(payload, language) or
                payload.get("translation_status", {}).get(language) != "auto_approved"):
            issues.append(language + "_verification_required")
        for cue in cues:
            if (not str((cue.get("translation") or {}).get(language) or "").strip() or
                    (cue.get("translation_review") or {}).get(language) != "auto_approved"):
                issues.append(language + "_cue_approval_required")
    review = payload.get("review")
    if isinstance(review, dict) and (review.get("quality_review") != "auto_approved" or
                                     review.get("audio_transcript_match_verified") is not True or
                                     review.get("rights_approved") is not True):
        issues.append("publication_review_required")
    return sorted(set(issues))


def accept_source(payload, result, job_id):
    updated = copy.deepcopy(payload)
    assessment = result.get("automatic_source_check") or {"passed": False, "issues": [{"reasons": ["verification_missing"]}]}
    # Alignment warnings and missing passages always override model agreement.
    passed = assessment.get("passed") and not result.get("exceptions") and not result.get("unmatched_spoken_runs")
    assessment = copy.deepcopy(assessment)
    assessment["issues"] = assessment.get("issues", []) + result.get("exceptions", []) + [
        {"reasons": ["possible_missing_words"], **run} for run in result.get("unmatched_spoken_runs", [])]
    # A gap is associated with the adjacent line so it does not force review of
    # every unrelated line. Technical verification failures remain file-wide.
    for issue in assessment["issues"]:
        when = issue.get("candidate_start", issue.get("start"))
        if not issue.get("paragraph") and isinstance(when, (int, float)) and updated.get("transcript"):
            issue["paragraph"] = min(range(len(updated["transcript"])), key=lambda i: abs(
                (updated["transcript"][i].get("start") or 0) - when)) + 1
    assessment["passed"] = bool(passed)
    report = updated.setdefault("automation", {})
    report.update({"version": VERSION, "source": assessment, "source_job_id": str(job_id)})
    if isinstance(assessment.get("duration_seconds"), (int, float)) and math.isfinite(assessment["duration_seconds"]):
        updated["media_duration_seconds"] = assessment["duration_seconds"]
    report.pop("translations", None)
    global_issue = any(not issue.get("paragraph") for issue in assessment["issues"])
    blocked = {issue["paragraph"] for issue in assessment["issues"] if issue.get("paragraph")}
    for index, cue in enumerate(updated.get("transcript", [])):
        before = copy.deepcopy(cue)
        if not cue.get("cue_id"):
            cue["cue_id"] = "auto-" + hashlib.sha256((str(job_id) + ":" + str(index)).encode()).hexdigest()[:24]
        # Isolate uncertain lines; other lines need no individual approval.
        if not global_issue and index + 1 not in blocked and assessment.get("verification_model") and cue.get("editorial_review", {}).get("decision") != "approved":
            cue["timing_status"] = "auto_approved"
            cue["editorial_review"] = {"decision": "auto_approved", "method": "second_recognizer_v1", "job_id": str(job_id)}
            if tokens(cue.get("text", "")) and cue.get("exercise_review") != "approved":
                cue["exercise_review"] = "auto_approved"
        elif cue.get("editorial_review", {}).get("decision") == "auto_approved":
            cue["editorial_review"] = {"decision": "draft"}
            cue["timing_status"] = "needs_review"
            cue["exercise_review"] = "draft"
        if cue != before:
            cue["review_revision"] = cue.get("review_revision", 0) + 1
    assessment["input"] = source_input(updated)
    updated["alignment_source"] = identity(updated)
    return updated


def apply_translations(payload, result, language, job_id):
    updated = copy.deepcopy(payload)
    cues = updated.get("transcript") or []
    seen = set()
    issues = []
    for row in result.get("translations", []):
        index = row["paragraph"] - 1
        if index < 0 or index >= len(cues) or index in seen or cues[index].get("text") != row["text"] or \
                (row.get("cue_id") and cues[index].get("cue_id") != row["cue_id"]) or \
                not row.get("translation", "").strip():
            raise ValueError("translation_source_changed")
        seen.add(index)
        cue = cues[index]
        before = copy.deepcopy(cue)
        translations = cue.setdefault("translation", {})
        review = cue.setdefault("translation_review", {})
        existing = translations.get(language, "").strip()
        if existing and existing != row["translation"] and not (
                review.get(language) == "draft" and
                (cue.get("translation_origin") or {}).get(language) == "machine" and
                updated.get("published") is False):
            raise ValueError("translation_would_overwrite_existing")
        if review.get(language) == "approved":
            continue
        translations[language] = row["translation"]
        if row.get("generated_by_worker"):
            cue.setdefault("translation_origin", {})[language] = "machine"
        quality = row.get("automatic_check") or {"passed": False}
        from translation_quality import candidate_risks
        candidate_issues = candidate_risks(row["text"], row["translation"], language)
        if candidate_issues:
            quality = {**quality, "passed": False, "reason": candidate_issues[0], "risks": candidate_issues}
        independently_verified = (quality.get("passed") is True and quality.get("independent_model") is True and
                                  (quality.get("calibration") or {}).get("passed") is True)
        other_language = "ckb" if language == "fa" else "fa"
        if translations[language].strip() == str(translations.get(other_language) or "").strip():
            independently_verified = False
            quality = {**quality, "reason": "language_substitution_suspected"}
        review[language] = "auto_approved" if independently_verified else "draft"
        if cue != before:
            cue["review_revision"] = cue.get("review_revision", 0) + 1
        if not independently_verified:
            issues.append({"paragraph": index + 1, "reasons": [quality.get("reason") or "translation_check_failed"]})
    for i, cue in enumerate(cues):
        if not cue.get("translation", {}).get(language) or cue.get("translation_review", {}).get(language) not in ACCEPTED:
            if not any(issue["paragraph"] == i + 1 for issue in issues):
                issues.append({"paragraph": i + 1, "reasons": ["translation_check_failed"]})
    updated.setdefault("translation_status", {})[language] = "auto_approved" if not issues else "draft"
    calibrations = [(row.get("automatic_check") or {}).get("calibration") or {} for row in result.get("translations", [])]
    calibration = calibrations[0] if calibrations and all(check == calibrations[0] for check in calibrations) else {"passed": False}
    updated.setdefault("automation", {}).setdefault("translations", {})[language] = {
        "version": VERSION, "job_id": str(job_id), "issues": issues, "passed": not issues,
        "calibration": calibration,
        "input": translation_input(updated, language)}
    review = updated.get("review")
    if isinstance(review, dict) and updated.get("automation", {}).get("source", {}).get("passed") and all(
            updated.get("translation_status", {}).get(target) in ACCEPTED for target in ("fa", "ckb")):
        review["quality_review"] = "auto_approved"
        review["audio_transcript_match_verified"] = True
    return updated
