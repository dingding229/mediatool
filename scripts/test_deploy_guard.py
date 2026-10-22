import copy
import itertools
import unittest
from deploy_guard import dependency_fingerprint


class DependencyFingerprintTest(unittest.TestCase):
    def setUp(self):
        self.inspect = {'Id': 'external', 'Image': 'sha256:original', 'Config': {'Env': ['OWNED=original']},
                        'Mounts': [{'Type': 'volume', 'Source': '/media', 'Destination': '/data', 'RW': True},
                                   {'Type': 'bind', 'Source': '/config', 'Destination': '/config', 'RW': False},
                                   {'Type': 'volume', 'Source': '/cache', 'Destination': '/cache', 'RW': True}],
                        'State': {'StartedAt': '2026-10-09T00:00:00Z'}}

    def test_all_mount_permutations_are_identical_without_mutating_receipt(self):
        original = copy.deepcopy(self.inspect)
        expected = dependency_fingerprint(self.inspect)
        for mounts in itertools.permutations(self.inspect['Mounts']):
            candidate = copy.deepcopy(self.inspect)
            candidate['Mounts'] = list(mounts)
            self.assertEqual(expected, dependency_fingerprint(candidate))
        self.assertEqual(original, self.inspect)

    def test_real_changes_are_not_normalized_away(self):
        mutations = [lambda x: x['Mounts'][0].update(Source='/other'),
                     lambda x: x['Mounts'][0].update(Destination='/other'),
                     lambda x: x['Mounts'][0].update(RW=False),
                     lambda x: x['Mounts'][0].update(Propagation='shared'),
                     lambda x: x['Mounts'].pop(),
                     lambda x: x['Config'].update(Env=['OWNED=changed']),
                     lambda x: x.update(Id='recreated'),
                     lambda x: x.update(Image='sha256:new'),
                     lambda x: x['State'].update(StartedAt='2026-10-23T00:00:00Z')]
        for mutate in mutations:
            with self.subTest(mutate=mutate):
                candidate = copy.deepcopy(self.inspect)
                mutate(candidate)
                self.assertNotEqual(dependency_fingerprint(self.inspect), dependency_fingerprint(candidate))


if __name__ == '__main__':
    unittest.main()
