"""Read-only Docker dependency fingerprint, shared by private deployment runners."""
import copy
import hashlib
import json


def dependency_fingerprint(inspect):
    """Ignore Docker's Mounts array order, never mount/configuration changes.

    Work on a copy: the caller may retain the original inspect receipt as evidence.
    Serialize every mount field (not only its sorting key), including RW/options.
    """
    value = copy.deepcopy({key: inspect[key] for key in ('Id', 'Image', 'Config', 'Mounts')})
    value['Mounts'].sort(key=lambda mount: json.dumps(mount, sort_keys=True, separators=(',', ':')))
    value['StartedAt'] = inspect['State']['StartedAt']
    return hashlib.sha256(json.dumps(value, sort_keys=True, separators=(',', ':')).encode()).hexdigest()
